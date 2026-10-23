# windows_exporter 插件化采集器

windows_exporter 是一个面向 Windows 系统的高性能 Prometheus 监控采集器，支持插件化扩展，方便用户根据实际需求开发和集成自定义采集器（插件）。

## 主要特性
- **插件化机制**：支持通过 my_collectors 目录和自动注册脚本，灵活扩展自定义采集器。
- **多平台支持**：支持 Windows/amd64 和 Windows/arm64 架构。
- **旧系统兼容**：额外发布 legacy 版本，支持 Windows 7 / Server 2008 R2 ~ 2012 R2。
- **自动化构建与发布**：集成 GitHub Actions 自动编译、发布、清理。
- **丰富的内置采集器**：覆盖常见 Windows 性能、服务、硬件、网络等监控需求。

## Windows 版本兼容性

`windows_exporter` 从 v0.30 起使用 Windows MI / Go 1.21+，不再支持 Windows 7 和
Server 2008 R2。本仓库因此同时发布两套资产（同一个 `latest` Release）：

| 资产 | 源码基线 | 适用系统 |
|------|----------|----------|
| `windows_exporter-amd64.exe` / `windows_exporter-arm64.exe` | 上游最新版（v0.32+） | Windows 10 / Server 2016 及以上 |
| `windows_exporter-amd64-legacy.exe` / `windows_exporter-arm64-legacy.exe` | 官方 v0.23.1 + Go 1.20.14 | Windows 7 SP1 / Server 2008 R2 SP1 / 8 / 8.1 / Server 2012 / 2012 R2 |

下载地址（`latest` 标签）：

```text
https://github.com/iflyelf/windows_exporter/releases/download/latest/windows_exporter-amd64.exe
https://github.com/iflyelf/windows_exporter/releases/download/latest/windows_exporter-amd64-legacy.exe
```

legacy 版本由 `.github/workflows/legacy.yml` 自动构建，并移植了本仓库的
`hardware_info` 采集器（WMI 实现），指标名与新版一致：

```powershell
# 默认采集器 + 硬件信息
windows_exporter-amd64-legacy.exe --collectors.enabled "[defaults],hardware_info"
```

或在 `config.yaml` 中：

```yaml
collectors:
  enabled: "[defaults],hardware_info"
```

> legacy 基于 v0.23.1，不包含 v0.30+ 新增的采集器/指标改名；在旧系统上建议只启用
> `cpu,cs,logical_disk,memory,net,os,service,system,textfile,hardware_info`。

### 构建 legacy 版本

```bash
git clone --depth 1 --branch v0.23.1 https://github.com/prometheus-community/windows_exporter.git legacy_src
cp my_collectors/legacy/hardware_info_collector.go legacy_src/collector/hardware_info.go
cd legacy_src
GOTOOLCHAIN=go1.20.14 CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -o windows_exporter-amd64-legacy.exe .
```

## 插件（自定义采集器）机制

- 所有自定义采集器（插件）建议放在 `my_collectors/` 目录下。
- 通过 `my_collectors/auto_register_collectors.sh` 脚本自动注册插件，无需手动修改主程序代码。
- 插件开发文档和示例可参考 `my_collectors/hardware_info_collector.md`。

### 插件开发与集成流程
1. 在 `my_collectors/` 目录下新建你的采集器 Go 文件（如 `my_plugin_collector.go`）。
2. 按照内置采集器的接口规范实现采集逻辑。
3. 运行 `my_collectors/auto_register_collectors.sh` 自动注册插件。
4. 重新编译 windows_exporter，插件会自动集成到主程序。

## 目录结构说明

```
windows_exporter/
├── my_collectors/           # 插件（自定义采集器）目录
│   ├── auto_register_collectors.sh  # 插件自动注册脚本
│   ├── hardware_info_collector.go   # 插件示例
│   └── ...
├── windows_exporter/        # 主程序源码
│   ├── internal/collector/  # 内置采集器
│   └── ...
├── .github/workflows/       # 自动化 CI/CD 配置
├── README.md                # 项目说明文档
└── ...
```

## 快速开始

1. **拉取代码**
   ```sh
   git clone https://github.com/iflyelf/windows_exporter.git
   cd windows_exporter
   ```
2. **开发自定义插件**
   - 在 `my_collectors/` 目录下新建 Go 文件，实现采集逻辑。
   - 运行 `bash my_collectors/auto_register_collectors.sh` 自动注册。
3. **编译主程序**
   ```sh
   cd windows_exporter
   go mod tidy
   go build -o windows_exporter.exe ./cmd/windows_exporter
   ```
4. **运行**
   ```sh
   ./windows_exporter.exe
   ```

## 贡献
欢迎提交自定义采集器插件或改进建议！

---

如需详细插件开发文档、接口说明或遇到问题，请查阅 `my_collectors/` 下的文档或在 Issues 区提问。