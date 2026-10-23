# legacy（Windows 7 / Server 2008 R2）采集器

本目录存放只能用于 **windows_exporter v0.23.1**（Go 1.20 构建）的采集器。

- `hardware_info_collector.go`：`my_collectors/hardware_info_collector.go` 的 WMI 移植版，
  指标名/标签保持一致。
- 不能把本目录文件直接用于仓库根目录的 v0.32 自动注册脚本；v0.32 请使用
  `my_collectors/hardware_info_collector.go`。
- CI 构建见 `.github/workflows/legacy.yml`：克隆上游 `v0.23.1`，把本文件复制为
  `collector/hardware_info.go`，再用 Go 1.20.14 编译 amd64/arm64。

启用方式：

```powershell
windows_exporter-amd64-legacy.exe --collectors.enabled "[defaults],hardware_info"
```
