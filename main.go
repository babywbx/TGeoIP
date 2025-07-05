// TGeoIP main application by wbx.
// Fetches Telegram's IP ranges, finds reachable IPs, and sorts them by country.

package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang"
	"inet.af/netaddr"
)

// Configuration Constants
const (
	// CidrListURL is the source for Telegram's official IP ranges.
	CidrListURL = "https://core.telegram.org/resources/cidr.txt"
	// MaxCheckers is the number of concurrent check operations.
	MaxCheckers = 200
	// CheckPort is the TCP port for connectivity tests.
	CheckPort = "443"
	// OutputFolder is the directory where result files are saved.
	OutputFolder = "geoip"
)

// geoRecord defines the structure for decoding country data from the MMDB.
type geoRecord struct {
	CountryCode string `maxminddb:"country_code"`
}

// main is the application entry point.
func main() {
	// Flag Definitions
	// Defines a -local flag for switching between execution modes.
	localMode := flag.Bool("local", false, "Enable local mode to use local DB file.")
	// Defines an -icmp flag to switch to ICMP ping mode.
	useICMP := flag.Bool("icmp", false, "Use ICMP ping instead of the default TCP check.")
	// Defines a -limit flag to limit the number of IPs to check.
	limit := flag.Int("limit", 0, "Limit the number of IPs to check (0 means no limit).")
	// Defines a -skip-check flag to skip the connectivity check.
	skipCheck := flag.Bool("skip-check", false, "Skip connectivity check and classify all expanded IPs.")
	flag.Parse()

	// Mode-dependent setup
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

	// Load GeoIP database
	log.Printf("Loading GeoIP database from: %s", dbPath)
	db, err := maxminddb.Open(dbPath)
	if err != nil {
		log.Fatalf("Fatal: Cannot open MMDB file: %v. In local mode, ensure 'ipinfo_lite.mmdb' is present.", err)
	}
	defer db.Close()

	// Main Execution Logic
	// Load CIDR list from source
	log.Println("Step 1: Loading CIDR list from source...")
	cidrs, err := loadCIDRs(CidrListURL)
	if err != nil {
		log.Fatalf("Fatal: Failed to load CIDR list: %v", err)
	}
	log.Printf("Successfully loaded %d IPv4 CIDR ranges.", len(cidrs))

	// Expand CIDRs to all host IPs
	log.Println("Step 2: Expanding CIDRs to all host IPs...")
	allIPs := expandCIDRsToIPs(cidrs)
	log.Printf("Expanded to %d total IPs to check.", len(allIPs))

	// Apply the IP limit if the -limit flag is used.
	if *limit > 0 && len(allIPs) > *limit {
		log.Printf(">>> Limiting check to the first %d IPs as per -limit flag. <<<", *limit)
		allIPs = allIPs[:*limit]
	}

	// Conditionally check for reachable IPs or use all of them.
	var ipsToProcess []string
	if *skipCheck {
		log.Println(">>> Skipping connectivity check as per -skip-check flag. <<<")
		ipsToProcess = allIPs
	} else {
		// Find reachable IPs
		log.Println("Step 3: Finding reachable IPs...")
		ipsToProcess = findReachableIPs(allIPs, *useICMP)
		log.Printf("Found %d reachable IPs.", len(ipsToProcess))
	}

	// Group IPs by country
	if len(ipsToProcess) > 0 {
		log.Println("Step 4: Grouping IPs by country...")
		countryMap := groupByCountryFromDB(ipsToProcess, db)
		log.Printf("Saving results for %d countries to the '%s/' directory.", len(countryMap), OutputFolder)
		saveResultsToFiles(countryMap)
	} else {
		log.Println("No IPs to process or save.")
	}

	// Save results to files
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

// loadCIDRs fetches the list of CIDRs from the specified URL.
func loadCIDRs(url string) ([]string, error) {
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

// findReachableIPs uses a worker pool to check for reachable IPs.
// It defaults to a reliable TCP check on port 443.
// If useICMP is true, it falls back to using the ICMP ping command.
// It includes a 3-try retry mechanism for both TCP and ICMP checks.
func findReachableIPs(ips []string, useICMP bool) []string {
	sem := make(chan struct{}, MaxCheckers)
	results := make(chan string, len(ips))
	var wg sync.WaitGroup

	if useICMP {
		// ICMP Ping Mode
		log.Printf("Checking %d IPs using ICMP ping with %d workers (up to 3 retries each)...", len(ips), MaxCheckers)
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
	} else {
		// Default TCP Check Mode with Retries
		log.Printf("Checking %d IPs on TCP port %s with %d workers (up to 3 retries each)...", len(ips), CheckPort, MaxCheckers)
		for _, ip := range ips {
			wg.Add(1)
			go func(ip string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				for i := 0; i < 3; i++ {
					address := net.JoinHostPort(ip, CheckPort)
					conn, err := net.DialTimeout("tcp", address, 2*time.Second)
					if err == nil {
						conn.Close()
						results <- ip
						return
					}
					time.Sleep(200 * time.Millisecond)
				}
			}(ip)
		}
	}

	wg.Wait()
	close(results)

	var reachableIPs []string
	for ip := range results {
		reachableIPs = append(reachableIPs, ip)
	}
	return reachableIPs
}

// saveResultsToFiles creates the output directory and saves all result files after sorting them.
func saveResultsToFiles(data map[string][]string) {
	os.MkdirAll(OutputFolder, 0755)
	for country, ipList := range data {
		// Sort the plain IP list before writing.
		sortIPStrings(ipList)
		filePath := fmt.Sprintf("%s/%s.txt", OutputFolder, country)
		writeLines(filePath, ipList)

		// Aggregate CIDRs from the IP list.
		cidrList := aggregateCIDRs(ipList)

		// Sort the resulting CIDR list before writing.
		sortCIDRStrings(cidrList)
		cidrPath := fmt.Sprintf("%s/%s-CIDR.txt", OutputFolder, country)
		writeLines(cidrPath, cidrList)
	}
}

// sortIPStrings sorts a slice of IP address strings numerically.
func sortIPStrings(ips []string) {
	sort.Slice(ips, func(i, j int) bool {
		ipA := net.ParseIP(ips[i])
		ipB := net.ParseIP(ips[j])
		if ipA == nil || ipB == nil {
			return ips[i] < ips[j] // Fallback to string sort if parsing fails
		}
		// Use To16() to ensure both IPv4 and IPv6 are compared correctly as 16-byte slices.
		return bytes.Compare(ipA.To16(), ipB.To16()) < 0
	})
}

// sortCIDRStrings sorts a slice of CIDR notation strings correctly.
func sortCIDRStrings(cidrs []string) {
	sort.Slice(cidrs, func(i, j int) bool {
		prefixA, errA := netaddr.ParseIPPrefix(cidrs[i])
		prefixB, errB := netaddr.ParseIPPrefix(cidrs[j])
		if errA != nil || errB != nil {
			return cidrs[i] < cidrs[j] // Fallback
		}
		// Compare IP addresses first, then prefix lengths
		ipCompare := prefixA.IP().Compare(prefixB.IP())
		if ipCompare != 0 {
			return ipCompare < 0
		}
		return prefixA.Bits() < prefixB.Bits()
	})
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
