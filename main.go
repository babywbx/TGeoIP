// TGeoIP main application by wbx.
// Fetches Telegram's IP ranges, finds reachable IPs, and sorts them by country.

package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"math/bits"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang"
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
	// Defines a -full flag to use both ICMP and TCP checks together.
	fullMode := flag.Int("full", 0, "Use both ICMP and TCP checks: 1=either passes, 2=both must pass.")
	flag.Parse()

	// Validate -full flag value
	if *fullMode != 0 && (*fullMode < 1 || *fullMode > 2) {
		slog.Error("Invalid -full value, only 1 or 2 allowed", "value", *fullMode)
		os.Exit(1)
	}

	// Mode-dependent setup
	var dbPath string
	if *localMode {
		slog.Info("Running in local mode")
		dbPath = "ipinfo_lite.mmdb"
	} else {
		slog.Info("Running in GitHub Actions mode")
		dbPath = os.Getenv("DB_PATH")
		if dbPath == "" {
			slog.Error("DB_PATH environment variable not set")
			os.Exit(1)
		}
	}

	// Load GeoIP database
	slog.Info("Loading GeoIP database", "path", dbPath)
	db, err := maxminddb.Open(dbPath)
	if err != nil {
		slog.Error("Cannot open MMDB file", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Main Execution Logic
	slog.Info("Step 1: Loading CIDR list from source")
	cidrs, err := loadCIDRs(CidrListURL)
	if err != nil {
		slog.Error("Failed to load CIDR list", "error", err)
		os.Exit(1)
	}
	slog.Info("Loaded IPv4 CIDR ranges", "count", len(cidrs))

	slog.Info("Step 2: Expanding CIDRs to all host IPs")
	allIPs := expandCIDRsToIPs(cidrs)
	slog.Info("Expanded IPs to check", "count", len(allIPs))

	// Apply the IP limit if the -limit flag is used.
	if *limit > 0 && len(allIPs) > *limit {
		slog.Info("Limiting IPs per -limit flag", "limit", *limit)
		allIPs = allIPs[:*limit]
	}

	// Conditionally check for reachable IPs or use all of them.
	var ipsToProcess []string
	if *skipCheck {
		slog.Info("Skipping connectivity check per -skip-check flag")
		ipsToProcess = allIPs
	} else {
		slog.Info("Step 3: Finding reachable IPs")
		if *fullMode > 0 {
			ipsToProcess = findReachableIPsFull(allIPs, *fullMode)
		} else {
			ipsToProcess = findReachableIPs(allIPs, *useICMP)
		}
		slog.Info("Found reachable IPs", "count", len(ipsToProcess))
	}

	// Group IPs by country
	if len(ipsToProcess) > 0 {
		slog.Info("Step 4: Grouping IPs by country")
		countryMap := groupByCountryFromDB(ipsToProcess, db)
		slog.Info("Saving results", "countries", len(countryMap), "folder", OutputFolder)
		saveResultsToFiles(countryMap)
	} else {
		slog.Info("No IPs to process or save")
	}

	slog.Info("Process completed successfully")
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
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
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
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			continue
		}
		prefix = prefix.Masked()
		hostBits := 32 - prefix.Bits()
		total := 1 << hostBits

		addr := prefix.Addr()
		if total > 2 {
			// Skip network and broadcast addresses
			addr = addr.Next()
			for i := 0; i < total-2; i++ {
				allIPs = append(allIPs, addr.String())
				addr = addr.Next()
			}
		} else {
			for i := 0; i < total; i++ {
				allIPs = append(allIPs, addr.String())
				addr = addr.Next()
			}
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
	var mu sync.Mutex
	var reachableIPs []string
	var wg sync.WaitGroup

	if useICMP {
		slog.Info("Checking IPs using ICMP ping", "count", len(ips), "workers", MaxCheckers)
		for _, ip := range ips {
			wg.Add(1)
			go func(ip string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				for i := 0; i < 3; i++ {
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-W", "2", ip)
					err := cmd.Run()
					cancel()
					if err == nil {
						mu.Lock()
						reachableIPs = append(reachableIPs, ip)
						mu.Unlock()
						return
					}
					time.Sleep(200 * time.Millisecond)
				}
			}(ip)
		}
	} else {
		slog.Info("Checking IPs on TCP port", "count", len(ips), "port", CheckPort, "workers", MaxCheckers)
		for _, ip := range ips {
			wg.Add(1)
			go func(ip string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				address := net.JoinHostPort(ip, CheckPort)
				for i := 0; i < 3; i++ {
					conn, err := net.DialTimeout("tcp", address, 3*time.Second)
					if err == nil {
						conn.Close()
						mu.Lock()
						reachableIPs = append(reachableIPs, ip)
						mu.Unlock()
						return
					}
					time.Sleep(200 * time.Millisecond)
				}
			}(ip)
		}
	}

	wg.Wait()
	return reachableIPs
}

// findReachableIPsFull performs both ICMP and TCP checks on each IP.
// fullMode: 1 = either ICMP or TCP passes, 2 = both must pass
func findReachableIPsFull(ips []string, fullMode int) []string {
	sem := make(chan struct{}, MaxCheckers)
	var mu sync.Mutex
	var reachableIPs []string
	var wg sync.WaitGroup

	var modeDesc string
	if fullMode == 1 {
		modeDesc = "either ICMP or TCP"
	} else {
		modeDesc = "both ICMP and TCP"
	}
	slog.Info("Checking IPs", "mode", modeDesc, "count", len(ips), "workers", MaxCheckers)

	for _, ip := range ips {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			icmpPassed := false
			tcpPassed := false

			// Check ICMP
			for i := 0; i < 3; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-W", "3", ip)
				err := cmd.Run()
				cancel()
				if err == nil {
					icmpPassed = true
					break
				}
				time.Sleep(200 * time.Millisecond)
			}

			// Check TCP
			address := net.JoinHostPort(ip, CheckPort)
			for i := 0; i < 3; i++ {
				conn, err := net.DialTimeout("tcp", address, 5*time.Second)
				if err == nil {
					conn.Close()
					tcpPassed = true
					break
				}
				time.Sleep(200 * time.Millisecond)
			}

			pass := false
			switch fullMode {
			case 1:
				pass = icmpPassed || tcpPassed
			case 2:
				pass = icmpPassed && tcpPassed
			}
			if pass {
				mu.Lock()
				reachableIPs = append(reachableIPs, ip)
				mu.Unlock()
			}
		}(ip)
	}

	wg.Wait()
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
// Pre-parses all IPs once to avoid O(N log N) re-parsing during sort.
func sortIPStrings(ips []string) {
	type ipEntry struct {
		addr netip.Addr
		orig string
	}
	entries := make([]ipEntry, len(ips))
	for i, s := range ips {
		a, _ := netip.ParseAddr(s)
		entries[i] = ipEntry{a, s}
	}
	slices.SortFunc(entries, func(a, b ipEntry) int {
		return a.addr.Compare(b.addr)
	})
	for i, e := range entries {
		ips[i] = e.orig
	}
}

// sortCIDRStrings sorts a slice of CIDR notation strings correctly.
func sortCIDRStrings(cidrs []string) {
	slices.SortFunc(cidrs, func(a, b string) int {
		prefixA, errA := netip.ParsePrefix(a)
		prefixB, errB := netip.ParsePrefix(b)
		if errA != nil || errB != nil {
			return strings.Compare(a, b)
		}
		if c := prefixA.Addr().Compare(prefixB.Addr()); c != 0 {
			return c
		}
		return prefixA.Bits() - prefixB.Bits()
	})
}

// aggregateCIDRs merges a list of IPs into the smallest possible set of CIDRs.
func aggregateCIDRs(ips []string) []string {
	if len(ips) == 0 {
		return nil
	}

	// Parse all IPs
	addrs := make([]netip.Addr, 0, len(ips))
	for _, s := range ips {
		if a, err := netip.ParseAddr(s); err == nil {
			addrs = append(addrs, a)
		}
	}
	if len(addrs) == 0 {
		return nil
	}

	// Sort and deduplicate
	slices.SortFunc(addrs, netip.Addr.Compare)
	addrs = slices.Compact(addrs)

	// Merge contiguous ranges into prefixes
	var result []string
	lo := addrToUint32(addrs[0])
	hi := lo
	for _, a := range addrs[1:] {
		v := addrToUint32(a)
		if v == hi+1 {
			hi = v
		} else {
			for _, p := range rangeToPrefixes(lo, hi) {
				result = append(result, p.String())
			}
			lo = v
			hi = v
		}
	}
	for _, p := range rangeToPrefixes(lo, hi) {
		result = append(result, p.String())
	}
	return result
}

// addrToUint32 converts a netip.Addr to a uint32 (IPv4 only).
func addrToUint32(a netip.Addr) uint32 {
	b := a.As4()
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

// uint32ToAddr converts a uint32 back to a netip.Addr (IPv4).
func uint32ToAddr(n uint32) netip.Addr {
	return netip.AddrFrom4([4]byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)})
}

// rangeToPrefixes decomposes a contiguous IP range [lo, hi] into CIDR prefixes.
func rangeToPrefixes(lo, hi uint32) []netip.Prefix {
	var prefixes []netip.Prefix
	cur := uint64(lo)
	end := uint64(hi)
	for cur <= end {
		// Alignment constraint: largest power-of-2 block at cur
		tz := 32
		if cur != 0 {
			tz = bits.TrailingZeros64(cur)
			if tz > 32 {
				tz = 32
			}
		}
		maxBits := tz
		// Don't exceed the remaining range
		for maxBits > 0 && cur+(1<<maxBits)-1 > end {
			maxBits--
		}
		prefixes = append(prefixes, netip.PrefixFrom(uint32ToAddr(uint32(cur)), 32-maxBits))
		cur += 1 << maxBits
	}
	return prefixes
}

// writeLines writes a slice of strings to a file without a trailing newline.
func writeLines(filePath string, lines []string) {
	if len(lines) == 0 {
		return
	}
	f, err := os.Create(filePath)
	if err != nil {
		slog.Error("Failed to create file", "path", filePath, "error", err)
		return
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for i, line := range lines {
		w.WriteString(line)
		if i < len(lines)-1 {
			w.WriteByte('\n')
		}
	}
	if err := w.Flush(); err != nil {
		slog.Error("Failed to write file", "path", filePath, "error", err)
	}
}
