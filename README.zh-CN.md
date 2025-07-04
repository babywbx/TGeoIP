<div align="center"><a name="readme-top"></a>

# 🗺️ TGeoIP

一个自动查找、分类并提供最新 Telegram 全球地理位置 IP 段的工具。

[English](./README.md) · **简体中文**

[![][automatically-update-TGeoIP-data]][automatically-update-TGeoIP-data-link]
[![][Last-updated-TGeoIP-data]][Last-updated-TGeoIP-data-link]
[![][github-license-shield]][github-license-link]

</div>

<details>
<summary><kbd>目录</kbd></summary>

- [📖 项目简介](#-项目简介)
- [✨ 功能特性](#-功能特性)
- [⚙️ 工作原理](#️-工作原理)
- [🚀 如何使用数据](#-如何使用数据)
- [🛠️ 本地开发](#️-本地开发)
  - [前置要求](#前置要求)
  - [运行程序](#运行程序)
  - [命令行参数](#命令行参数)
- [🔧 配置](#-配置)
- [🤝 参与贡献](#-参与贡献)
- [📄 许可证](#-许可证)

</details>

## 📖 项目简介

TGeoIP 是一个自动化工具，它能自动获取 Telegram 最新的官方 IP 段，检测其中的可用主机，并按地理位置进行分类。最终生成的 IP 列表和 CIDR 网段会自动提交到 `geoip` 分支，方便直接使用。

本项目的目标是为开发者和网络管理员提供一个持续更新、可靠的 Telegram IP 分类数据源。

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## ✨ 功能特性

- **🤖 完全自动化**: 通过 GitHub Actions 每小时自动更新。
- **⚡️ 高效并发**: 使用高并发检测，快速处理数千个 IP。
- **🛡️ 可靠性高**: 默认使用 TCP 443 端口检测，在云环境中比 ICMP ping 更可靠。
- **🌍 地理位置查询**: 使用本地 MMDB 数据库，查询速度快且支持离线。
- **📝 双格式输出**: 同时生成纯 IP 列表 (`US.txt`) 和聚合后的 CIDR 列表 (`US-CIDR.txt`)。

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## ⚙️ 工作原理

1.  GitHub Actions 工作流按每小时计划自动运行。
2.  它会下载最新的 Telegram CIDR 列表和免费的 IPinfo 地理位置数据库。
3.  Go 应用程序处理所有 IP，检测存活主机。
4.  结果按国家分组并保存为 `.txt` 文件。
5.  `github-actions[bot]` 机器人自动将更新后的文件提交到 `geoip` 分支。

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## 🚀 如何使用数据

所有生成的数据都位于本仓库的 `geoip` 分支。这个分支**只包含**数据文件，方便集成。

**[➡️ 前往 `geoip` 分支查看数据][geoip-branch-link]**

你可以直接在你的防火墙、路由规则或其他应用中使用这些文件。

<div align="right">

[![][back-to-top]](#readme-top)

</div>

## 🛠️ 本地开发

### 前置要求
要在本地运行此程序，你需要：
- Go (推荐版本 1.24+)
- 从 [IPinfo][ipinfo-lite-link] 下载的 `ipinfo_lite.mmdb` 文件，并放置于项目根目录。

### 运行程序
**克隆仓库并运行：**

```bash
# 使用默认的 TCP 检测模式运行
go run . -local

# 限制只测试前 1000 个 IP，用于快速验证
go run . -local -limit 1000

# 使用 ICMP ping 模式运行
go run . -local -icmp
```

### 命令行参数
-local: 启用本地模式（会从当前目录读取 ipinfo_lite.mmdb）。

-icmp: 将检测方法从默认的 TCP 连接切换为 ICMP ping。

-limit <数量>: 限制要检测的 IP 数量 (例如 -limit 500)。0 代表不限制。

<div align="right">

[![][back-to-top]](#readme)

</div>

## 🔧 配置
要让 GitHub Actions 工作流正常运行，你必须在仓库的 (`Settings` > `Secrets and variables` > `Actions`) 中设置一个 Secret：

- IPINFO_TOKEN: 你在 [ipinfo.io][ipinfo-link] 的 API Token，用于下载 MMDB 数据库。

<div align="right">

[![][back-to-top]](#readme)

</div>

## 🤝 参与贡献
欢迎任何形式的贡献！你可以随时提交 Issue 或 Pull Request。

<div align="right">

[![][back-to-top]](#readme)

</div>

## 📄 许可证
本项目采用 GNU 通用公共许可证 v3.0 (GPLv3) 授权。详情请见 LICENSE 文件。

<div align="right">

[![][back-to-top]](#readme)

</div>

Copyright © 2025 Babywbx.

<!-- LINK GROUP -->

[automatically-update-TGeoIP-data]: https://img.shields.io/github/actions/workflow/status/babywbx/TGeoIP/update-geoip.yml?label=%E8%87%AA%E5%8A%A8%E6%9B%B4%E6%96%B0%20TGeoIP%20%E6%95%B0%E6%8D%AE&labelColor=black&logo=githubactions&logoColor=white&style=flat-square
[automatically-update-TGeoIP-data-link]: https://github.com/babywbx/TGeoIP/actions/workflows/update-geoip.yml
[Last-updated-TGeoIP-data]: https://img.shields.io/github/last-commit/babywbx/TGeoIP/geoip?label=TGeoIP%20%E6%95%B0%E6%8D%AE%E6%9C%80%E5%90%8E%E6%9B%B4%E6%96%B0%E6%97%B6%E9%97%B4&labelColor=black&logo=githubactions&logoColor=white&style=flat-square
[Last-updated-TGeoIP-data-link]: https://github.com/babywbx/TGeoIP/tree/geoip
[github-license-link]: https://github.com/babywbx/TGeoIP/blob/main/LICENSE
[github-license-shield]: https://img.shields.io/github/license/babywbx/TGeoIP?style=flat-square&logo=gplv3&label=%E8%AE%B8%E5%8F%AF%E8%AF%81&labelColor=black&color=white
[back-to-top]: https://img.shields.io/badge/-BACK_TO_TOP-151515?style=flat-square
[geoip-branch-link]: https://github.com/babywbx/TGeoIP/tree/geoip
[ipinfo-lite-link]: https://ipinfo.io/lite
[ipinfo-link]: https://ipinfo.io
