# TaierSpeedtest

全球网测（泰尔测速）Linux 客户端。Fork 自 [MiaM1ku/taierspeedtest](https://github.com/MiaM1ku/taierspeedtest)，新增 IPv4 / IPv6 选择。协议还原自 `com.cnspeedtest.globalspeed` 4.4.8。

单个静态 Go 二进制，不依赖系统字体或 ImageMagick。默认交互：北上广省会 × 电信/联通/移动，单线程 + 多线程对照。本机 IPv6 能访问互联网时自动加测 IPv6。

## 一键运行

任意 Linux amd64/arm64 机器，无需预装依赖：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/MbaoWang/taierspeedtest-mod/main/run.sh)
```

指定测速点：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/MbaoWang/taierspeedtest-mod/main/run.sh) --points 湖北
```

只测 IPv6（`--ip 4` / `--ip 6` / `--ip both`）：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/MbaoWang/taierspeedtest-mod/main/run.sh) --ip 6
```

`run.sh` 只下载对应架构的 Release 并执行。二进制内嵌中文字体，可直接生成 PNG。

## 本地编译

```bash
git clone https://github.com/MbaoWang/taierspeedtest-mod.git
cd taierspeedtest-mod
go build -o taierspeedtest .
./taierspeedtest
```

## 交互

直接运行后按提示输入：

- **测速点**：回车 = 北上广省会三网；也可 `湖北`、`北京,上海,浙江`、`武汉电信,杭州联通`
- **模式**：回车 = 单线程 + 多线程对照
- **IP 协议**：回车 = 本机 IPv6 可用时 IPv4 + IPv6 都测，否则只测 IPv4；也可选只测 IPv4 或只测 IPv6

## 指定 IPv4 / IPv6

```bash
./taierspeedtest --ip 4      # 只测 IPv4
./taierspeedtest --ip 6      # 只测 IPv6
./taierspeedtest --ip both   # IPv4 + IPv6 都测
```

不加 `--ip` 时，交互模式会询问；非交互模式默认 IPv4，并在 IPv6 可用时自动加测。旧参数 `--ipv6`（= both）、`--no-ipv6`（= 4）仍可用。

## 结果说明

| 栏位 | 含义 |
|---|---|
| 去程 | 测速点 → 本机（下载） |
| 回程 | 本机 → 测速点（上传） |
| 回程重传 | 单线程回程测速时，本机 TCP 重传段数 ÷ 发出数据段数（读取 Linux `TCP_INFO` 的 `total_retrans / data_segs_out`，算法参考 [TcpQuality](https://github.com/ibsgss/TcpQuality)）。只测多线程时改用多线程回程。≤10% 绿、10–20% 黄、≥20% 红 |

测速失败时会自动重测（`--retries`，0–3，默认 1）：任一方向测到 0 就等 3 秒换新 key 重跑该模式，保留失败最少的一次，终端会标示「已自动重测 ×N」。单线程与多线程各自取一把新的测速 key，避免同一把 key 连跑四段时多线程回程被服务器拒绝。

测速前须向服务器排队取 key：遇到「排队中」或「服务器忙」时，会每秒询问一次，最多等 `--queue-wait` 秒（默认 15）。仍拿不到时该行显示 `-` 并在行尾注明原因（服务器忙 / 排队超时 / 连不上测速服务器 / 找不到测速节点）。

排错：加 `--debug` 会印出每个测速点的候选节点、选用节点、排队过程与各阶段速度。

去程方向的重传发生在测速服务器端，本机无法读取，因此只提供回程重传。

## 结果图

测速结束后生成 PNG，并按以下顺序上传：

1. [111666.best](https://111666.best/)（与 NodeQuality 相同图床，仅接受位图）
2. catbox
3. 0x0.st

全部失败时打印本地 PNG 路径。图中只显示城市和运营商，不包含 IP。

## 发布

推送版本标签后，GitHub Actions 会交叉编译 Linux amd64/arm64 并更新 Release：

```bash
git tag v1.0.3
git push origin v1.0.3
```

## 说明

- 延迟优先 ICMP Ping，否则 HTTP tcping
- 仅供个人网络质量测试
