<div align="center"><a name="readme-top"></a>

# 🗺️ TGeoIP

A tool to automatically find and categorize Telegram's IP ranges by geolocation.

**English** · [简体中文](./README.zh-CN.md)

[![][automatically-update-TGeoIP-data]][automatically-update-TGeoIP-data-link]
[![][Last-updated-TGeoIP-data]][Last-updated-TGeoIP-data-link]
[![][github-license-shield]][github-license-link]

</div>

<details>
<summary><kbd>Table of Contents</kbd></summary>

- [📖 About The Project](#-about-the-project)
- [✨ Features](#-features)
- [⚙️ How It Works](#️-how-it-works)
- [🚀 How to Use the Data](#-how-to-use-the-data)
- [🛠️ Local Development](#️-local-development)
  - [Prerequisites](#prerequisites)
  - [Running the Application](#running-the-application)
  - [Command-line Flags](#command-line-flags)
- [🔧 Configuration](#-configuration)
- [🤝 Contributing](#-contributing)
- [📄 License](#-license)

</details>

## 📖 About The Project

TGeoIP is an automated tool that fetches the latest official IP ranges from Telegram, checks for reachable hosts, and categorizes them by geolocation. The resulting IP lists and CIDR blocks are automatically committed to the `geoip` branch for easy use.

This project aims to provide an up-to-date, reliable source of categorized Telegram IPs for developers and network administrators.

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## ✨ Features

- **🤖 Fully Automated**: Updates hourly via GitHub Actions.
- **⚡️ Fast & Concurrent**: High-concurrency checks with memory-efficient `net/netip` based IP processing, pre-parsed sorting, and streaming file I/O.
- **🪶 Lightweight**: Only one external dependency (`maxminddb-golang`). Core IP logic is pure Go stdlib.
- **🛡️ Reliable**: Defaults to a TCP port 443 check with HTTP timeouts, more reliable than ICMP ping in cloud environments.
- **🌍 Geolocation Lookup**: Uses a local MMDB database for fast and offline geo-lookups.
- **📝 Dual-Format Output**: Generates both plain IP lists (`US.txt`) and aggregated CIDR lists (`US-CIDR.txt`).
- **🔄 Retry Mechanism**: Implements 3-retry logic with 200ms intervals for better reliability.
- **⏱️ Optimized Timeouts**: Uses 3-second timeouts for checks, 30-second timeout for HTTP requests.
- **🔍 Dual Check Modes**: Support for ICMP-only, TCP-only, or combined ICMP/TCP checks.
- **⚡ Skip Check Option**: Bypass connectivity checks for faster processing when needed.

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## ⚙️ How It Works

1.  A GitHub Actions workflow runs on an hourly schedule.
2.  It downloads the latest Telegram CIDR list and the free IPinfo geo database.
3.  The Go application processes all IPs, checking for live hosts.
4.  Results are grouped by country and saved as `.txt` files.
5.  `wbxBot` automatically commits the updated files to the `geoip` branch.

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## 🚀 How to Use the Data

The generated data is available on the `geoip` branch of this repository. This branch contains **only** the data files for easy integration.

**[➡️ Go to the `geoip` branch to view the data][geoip-branch-link]**

You can use these files directly in your firewall, routing rules, or other applications.

<div align="right">

[![][back-to-top]](#readme)

</div>

## 🛠️ Local Development

### Prerequisites
To run this application locally, you need:
- Go (version 1.26+ recommended)
- An `ipinfo_lite.mmdb` file from [IPinfo][ipinfo-lite-link] in the project root.

### Running the Application
**Clone the repository and run:**

```bash
# Run with default TCP check
go run . -local

# Run with a limit of 1000 IPs for a quick test
go run . -local -limit 1000

# Run using the ICMP ping method
go run . -local -icmp

# Skip connectivity checks for faster processing
go run . -local -skip-check

# Use dual ICMP/TCP check mode (either passes)
go run . -local -full 1

# Use dual ICMP/TCP check mode (both must pass)
go run . -local -full 2

# Combine multiple flags for specific use cases
go run . -local -full 1 -limit 500
```

### Command-line Flags

- `-local`: Enables local mode (uses `ipinfo_lite.mmdb` from the current directory).
- `-icmp`: Switches the check method from the default TCP dial to ICMP ping.
- `-limit <number>`: Limits the number of IPs to check (e.g., `-limit 500`). `0` means no limit.
- `-skip-check`: Skips connectivity checks and classifies all expanded IPs (useful for faster processing).
- `-full <mode>`: Uses both ICMP and TCP checks together:
  - `-full 1`: Either ICMP or TCP passes (more lenient)
  - `-full 2`: Both ICMP and TCP must pass (more strict)

<div align="right">

[![][back-to-top]](#readme)

</div>

## 🔧 Configuration

For the GitHub Actions workflow to run, you must set one secret in your repository settings (`Settings` > `Secrets and variables` > `Actions`):

- `IPINFO_TOKEN`: Your API token from [ipinfo.io](https://ipinfo.io), which is required to download the MMDB database.

<div align="right">

[![][back-to-top]](#readme)

</div>

## 🤝 Contributing

Contributions are welcome! Please feel free to open an issue or submit a pull request.

<div align="right">

[![][back-to-top]](#readme)

</div>

## 📄 License

This project is licensed under the GNU General Public License v3.0. See the `LICENSE` file for details.

<div align="right">

[![][back-to-top]](#readme)

</div>

Copyright © 2025-2026 Babywbx.

<!-- LINK GROUP -->

[automatically-update-TGeoIP-data]: https://img.shields.io/github/actions/workflow/status/babywbx/TGeoIP/update-geoip.yml?label=Automatically%20update%20TGeoIP%20data&labelColor=black&logo=githubactions&logoColor=white&style=flat-square
[automatically-update-TGeoIP-data-link]: https://github.com/babywbx/TGeoIP/actions/workflows/update-geoip.yml
[Last-updated-TGeoIP-data]: https://img.shields.io/github/last-commit/babywbx/TGeoIP/geoip?label=Last%20updated%20TGeoIP%20data&labelColor=black&logo=githubactions&logoColor=white&style=flat-square
[Last-updated-TGeoIP-data-link]: https://github.com/babywbx/TGeoIP/tree/geoip
[github-license-link]: https://github.com/babywbx/TGeoIP/blob/main/LICENSE
[github-license-shield]: https://img.shields.io/github/license/babywbx/TGeoIP?style=flat-square&logo=gplv3&labelColor=black&color=white
[back-to-top]: https://img.shields.io/badge/-BACK_TO_TOP-151515?style=flat-square
[geoip-branch-link]: https://github.com/babywbx/TGeoIP/tree/geoip
[ipinfo-lite-link]: https://ipinfo.io/lite
[ipinfo-link]: https://ipinfo.io
