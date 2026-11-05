# AndroMeld Guardian — KernelSU + WebUI (test build 0.1.0)

Android 本机、arm64、KernelSU。不是 Aether Extension，不需要 Termux/Python/Node 在后台运行，也不是电脑 ADB 服务。

## 下载与上手

- 源码：[eric1932/andromeld-guardian](https://github.com/eric1932/andromeld-guardian)
- 安装包：[Releases](https://github.com/eric1932/andromeld-guardian/releases)（公开下载）。
- KernelSU 中安装 Release 的 arm64 ZIP；若显示待更新/待重启，先由用户方便时重启手机。
- KernelSU → 模块 → AndroMeld Guardian → WebUI → 开启「自动保护」并确认。默认1分钟检查即可。
- 关闭面板后后台仍可运行；想停止自动修复，关掉「自动保护」即可，不会主动断开现有会话。
- **仅下载/安装、模块显示 enabled，均不等于已经在保护。** 要核对是否已生效、worker状态、当前模式及状态更新时间。

详见 [测试与未验证范围](TESTING.md)。发布仓库不含APK、诊断包、设备状态或密钥。

## 用途与界限

- **默认观察**：只检查状态；在 WebUI 中明确启用“保护”才自动恢复。
- 支持已核对的 AndroMeld **2.0.0 / build26101400**。安装不同版本时停止自动变更并显示 blocked；不是旧 Skill 仅核实的 build26092917。
- 保护 Web 入口和**已配对 Mac 的 MacLink**；绝不长期开放 `pairing.toggle`/Mac 配对入口。
- 没有客户端连接是正常状态，不会据此重启 daemon。
- 无连接时大约每3分钟通过应用本身的唤醒处理链续期 idle 窗口；不开独立公网/本地 HTTP 服务、不发云端推送、不修改 SharedPreferences、idle常量、账号或配对数据。
- daemon 确认消失且连续3次检查缺失时，尝试按当前APK启动官方 `DeviceAgentMain`。要求之前核验过其 root 身份、保存过既有公开证书摘要，且既有数据文件符合验证条件。**单实例锁交给应用本身，不删除锁**。
- 活进程管理接口卡住时报告异常，不强杀、不自动重启，避免破坏正在进行的连接。没有“永不断线”承诺：网络/TURN/浏览器/服务器故障仍可能导致 ICE 失败。

## 运行方式

模块本体包括静态依赖的 Android arm64 Go PIE（约4MB，解释器 `/system/bin/linker64`，不依赖 Termux动态库）、`service.sh` 和 `webroot`。

- 通过 **KernelSU 官方模块安装器**安装ZIP。支持热安装的管理器会立即应用 WebUI 并启动观察进程；不支持时由安装器提示下次重启生效，不替用户重启手机。
- 在 KernelSU → 模块 → AndroMeld Guardian → WebUI 打开面板。
- 首次切换保护需要确认远程入口持续可用的耗电/暴露风险；用户随后在 AndroMeld 中手动关闭入口，保护模式会再次开启。想手动关闭前，先把守护切回观察。
- 切回观察仅停止自动恢复，不断开当前会话。停用/移除模块后，worker看到标志会自行退出；卸载脚本只停止自己的worker。
- 不持有唤醒锁、不设置精准alarm、不改系统省电/SELinux/其他应用cgroup。手机深睡可能延后检查，因此不是精确定时保活。

## 安全与持久化

- 仅主用户 helper；每次动态获取包UID、APK路径与版本，不复用历史PID/UID。
- 管理TCP只允许`web.status`和`mac.link`。短命原生子进程从root降到helper UID后连接；父进程验证原PID/UID/startticks/state、监听和该条ESTABLISHED socket归属后，才放行只读请求；回包后再次检查同一目标。
- 自动唤醒只发送本机 root 显式广播到固定的 FirebaseInstanceIdReceiver，固定`t=open_web_service`或`t=open_mac_link`，由 AndroMeld 自己执行 consent/发现模式门禁、网关/prefs/FGS同步和失败回滚。这**不是**测试真实FCM云端链路。
- 恢复不成功或回包未知时，先持久化的尝试记录保留冷却，防止重试风暴。每通道3分钟冷却/每小时最多24次；冷启动10分钟冷却/每小时最多3次。历史损坏、异常缺失或解析失败时，保护模式只观察、不覆盖成空历史；需先切观察并做一次检查，再重新确认保护。
- 在昂贵查询完成后、真正发广播或启动进程的紧前再次检查保护策略和stop/disable/remove标志。已经交给系统执行的调用无法撤回，但不会再发下一条恢复请求。
- 已有设备上历史数据目录和identity可由shell(2000)拥有、daemon为root。只允许root/shell的私有常规文件，拒绝符号链接与宽松权限，**不chown/chmod应用数据**。冷启动前比较已记录的公开证书SHA256，避免在未知身份上重新登记。
- `/data/adb/andromeld_guardian`（0700）保存配置、公开证书摘要、状态和最多60条分类事件（文件0600），没有私钥/token/客户端名称/原始应用日志。
- 每次安装/升级（热安装和重启后生效两条路径）都会在新模块中写入`reauthorize`标记，强制观察；只有用户明确切换保护时才移除此标记。热升级还会先停止自己的worker；不自动延续新代码的修复授权。停止若超时，stop标记仍持续禁止变更，后续start会先落盘观察才清除标记。卸载为避免并发删除竞态保留上述小型私有数据目录，确认进程停止后可手动删除；重装默认的热安装脚本仍会重置为观察。
- 控制面板只有固定命令，无任意shell输入、CDN、远端脚本、网络请求或HTML注入。

## CLI（root，Android本机）

```text
/data/adb/modules/andromeld_guardian/bin/guardian view
/data/adb/modules/andromeld_guardian/bin/guardian tick
/data/adb/modules/andromeld_guardian/bin/guardian policy observe
/data/adb/modules/andromeld_guardian/bin/guardian policy protect
/data/adb/modules/andromeld_guardian/bin/guardian interval 60
/data/adb/modules/andromeld_guardian/bin/guardian stop
/data/adb/modules/andromeld_guardian/bin/guardian start
```

`view`是缓存，必须看`checked_at`和worker状态。`tick`为一次实时检查，保护模式下可能恢复入口；默认观察不会变更AndroMeld。`stop`停自己的worker并切回观察，绝不杀AndroMeld。

## 构建与测试

开发源码在`src/`，只用Go标准库，不联网下载依赖。推荐在 Android arm64 的 Termux 开发环境执行 `sh scripts/build.sh`：先测试、再编译和打包；此脚本不需要root，也不会安装或操作AndroMeld。开发时需要Go和Python3，运行模块不需要它们。

前端离线测试在有Node的开发机执行 `node tests/webui.test.cjs`；无Android/root操作。

等价的手工编译步骤（Termux当前Go android/arm64）：

```sh
mkdir -p .gotmp
cd src
TMPDIR="$PWD/../.gotmp" GOTMPDIR="$PWD/../.gotmp" GOMAXPROCS=2 go test -p 1 -v ./...
TMPDIR="$PWD/../.gotmp" GOTMPDIR="$PWD/../.gotmp" CGO_ENABLED=0 go build \
  -buildmode=pie -trimpath -tags netgo,osusergo -ldflags='-s -w -buildid=' \
  -o ../module/bin/guardian .
```

安装包仅包含`module/`内容，不含`research/`中的APK反汇编或设备现场证据。不要把开发时的分析/日志资料上传到公开仓库。

## 验证等级

- 新版本唤醒流程：APK字节码核对 + 本机真实 Web开启/prefs同步/FGS前台 + MacLink active，期间原daemon未重启、pairing仍false。
- 新原生状态查询及UID/socket门禁：在本机实际只读检查成功。
- 恢复策略、速率限制、未知版本、缺失字段、隐私过滤、TCP帧及socket校验：离线单元测试。
- **冷启动恢复只验证实现/离线保护；未为测试故意杀死用户daemon。** 锁屏长期可靠性、开机启动和真实远端自动重连，必须后续实际验收，不能用“安装成功”替代。

官方参考：[模块机制](https://kernelsu.org/guide/module.html)、[WebUI](https://kernelsu.org/guide/module-webui.html)。本模块不加载sepolicy.rule、system.prop、initrc、preinit，不修改系统分区。
