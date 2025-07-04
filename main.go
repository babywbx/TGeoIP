// TGeoIP main application.
// Fetches Telegram's IP ranges, finds reachable IPs, and sorts them by country.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang"
	"inet.af/netaddr"
)

// --- Configuration Constants ---
const (
	// CidrListURL is the source for Telegram's official IP ranges.
	CidrListURL = "https://core.telegram.org/resources/cidr.txt"
	// DBDownloadURL is the template URL for the IPinfo MMDB database.
	DBDownloadURL = "https://ipinfo.io/data/free/country.mmdb?token=%s"
	// MaxPingWorkers is the number of concurrent ping operations.
	MaxPingWorkers = 200
	// OutputFolder is the directory where result files are saved.
	OutputFolder = "geoip"
)

// geoRecord defines the structure for decoding country data from the MMDB.
type geoRecord struct {
	CountryCode string `maxminddb:"country_code"`
}

// main is the application entry point.
func main() {
	// Defines a -local flag for switching between execution modes.
	localMode := flag.Bool("local", false, "Enable local mode to use local DB file.")
	flag.Parse()

	var dbPath string
	if *localMode {
		log.Println("--- Running in Local Mode ---")
		dbPath = "ipinfo_lite.mmdb" // Use local DB file.
	} else {
		log.Println("--- Running in GitHub Actions Mode ---")
		dbPath = os.Getenv("DB_PATH") // Use DB path from environment variable.
		if dbPath == "" {
			log.Fatalf("Fatal: DB_PATH environment variable not set.")
		}
	}

	log.Printf("Loading GeoIP database from: %s", dbPath)
	db, err := maxminddb.Open(dbPath)
	if err != nil {
		log.Fatalf("Fatal: Cannot open MMDB file: %v. In local mode, ensure 'ipinfo_lite.mmdb' is present.", err)
	}
	defer db.Close()

	log.Println("Step 1: Downloading CIDR list...")
	cidrs, err := downloadCIDRs(CidrListURL)
	if err != nil {
		log.Fatalf("Fatal: Failed to download CIDR list: %v", err)
	}
	log.Printf("Loaded %d IPv4 CIDR ranges.", len(cidrs))

	log.Println("Step 2: Expanding CIDRs to all host IPs...")
	allIPs := expandCIDRsToIPs(cidrs)
	log.Printf("Expanded to %d total IPs to check.", len(allIPs))

	log.Println("Step 3: Finding reachable IPs...")
	reachableIPs := pingIPs(allIPs)
	log.Printf("Found %d reachable IPs.", len(reachableIPs))

	if len(reachableIPs) > 0 {
		log.Println("Step 4: Grouping IPs by country...")
		countryMap := groupByCountryFromDB(reachableIPs, db)
		log.Printf("Saving results for %d countries to the '%s/' directory.", len(countryMap), OutputFolder)
		saveResultsToFiles(countryMap)
	} else {
		log.Println("No reachable IPs found, nothing to save.")
	}

	log.Println("Process completed successfully.")
}

// groupByCountryFromDB looks up IPs in the local MMDB and groups them by country code.
func groupByCountryFromDB(ips []string, db *maxminddb.Reader) map[string][]string {
	countryMap := make(map[string][]string)
	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		var record geoRecord
		err := db.Lookup(ip, &record)
		if err == nil && record.CountryCode != "" {
			countryMap[record.CountryCode] = append(countryMap[record.CountryCode], ipStr)
		}
	}
	return countryMap
}

// downloadCIDRs fetches the list of CIDRs from the specified URL.
func downloadCIDRs(url string) ([]string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %s", resp.Status)
	}
	var ipv4Cidrs []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.Contains(line, ":") {
			ipv4Cidrs = append(ipv4Cidrs, line)
		}
	}
	return ipv4Cidrs, scanner.Err()
}

// expandCIDRsToIPs iterates through CIDR strings and returns all host IPs within them.
func expandCIDRsToIPs(cidrs []string) []string {
	var allIPs []string
	for _, cidr := range cidrs {
		ip, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		var currentIPs []string
		for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); incrementIP(ip) {
			ipCopy := make(net.IP, len(ip))
			copy(ipCopy, ip)
			currentIPs = append(currentIPs, ipCopy.String())
		}
		if len(currentIPs) > 2 {
			allIPs = append(allIPs, currentIPs[1:len(currentIPs)-1]...)
		} else {
			allIPs = append(allIPs, currentIPs...)
		}
	}
	return allIPs
}

// pingIPs uses a worker pool to ping IPs concurrently with a retry mechanism.
func pingIPs(ips []string) []string {
	sem := make(chan struct{}, MaxPingWorkers)
	results := make(chan string, len(ips))
	var wg sync.WaitGroup
	log.Printf("Pinging %d IPs with %d workers (up to 3 retries each)...", len(ips), MaxPingWorkers)
	for _, ip := range ips {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			for i := 0; i < 3; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-W", "1", ip)
				if err := cmd.Run(); err == nil {
					results <- ip
					return
				}
				time.Sleep(200 * time.Millisecond)
			}
		}(ip)
	}
	wg.Wait()
	close(results)
	var reachableIPs []string
	for ip := range results {
		reachableIPs = append(reachableIPs, ip)
	}
	return reachableIPs
}

// saveResultsToFiles creates the output directory and saves all result files.
func saveResultsToFiles(data map[string][]string) {
	os.MkdirAll(OutputFolder, 0755)
	for country, ipList := range data {
		filePath := fmt.Sprintf("%s/%s.txt", OutputFolder, country)
		writeLines(filePath, ipList)
		cidrPath := fmt.Sprintf("%s/%s-CIDR.txt", OutputFolder, country)
		writeLines(cidrPath, aggregateCIDRs(ipList))
	}
}

// aggregateCIDRs merges a list of IPs into the smallest possible set of CIDRs.
func aggregateCIDRs(ips []string) []string {
	var builder netaddr.IPSetBuilder
	if ips == nil {
		return nil
	}
	for _, ipStr := range ips {
		if ip, err := netaddr.ParseIP(ipStr); err == nil {
			builder.Add(ip)
		}
	}
	ipSet, _ := builder.IPSet()
	if ipSet == nil {
		return nil
	}
	var cidrs []string
	ranges := ipSet.Ranges()
	for _, r := range ranges {
		prefixes := r.Prefixes()
		for _, p := range prefixes {
			cidrs = append(cidrs, p.String())
		}
	}
	return cidrs
}

// incrementIP treats an IP address as a big-endian integer and increments it by one.
func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// writeLines writes a slice of strings to a file without a trailing newline.
func writeLines(filePath string, lines []string) {
	if len(lines) == 0 {
		return
	}
	output := strings.Join(lines, "\n")
	err := os.WriteFile(filePath, []byte(output), 0644)
	if err != nil {
		log.Printf("Error writing to file %s: %v", filePath, err)
	}
}
