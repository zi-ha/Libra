# Libra · 轻量电子书管理系统

> 运行在 NAS 上的自托管电子书库。挂载目录 → 自动扫描 EPUB / PDF → 提取元数据与封面 → 封面墙浏览 → 在线阅读信息 / 下载原文件。

单二进制 + 单 SQLite 文件，零外部服务依赖；前端零构建、零 CDN，断网也能正常使用。

![首页深色主题](docs/screenshots/home-dark.webp)

## 功能

- **媒体库**：任意多个媒体库，每个库对应一个本地目录；独立扫描、独立统计、独立图标。
- **媒体库封面**：管理员可在「编辑媒体库」里上传自定义封面；不上传时自动取该库最近几本书的封面，斜向叠放生成一张拼贴封面。
- **首页**：精选轮播（统一 `cover` + 高斯模糊 + 暗色遮罩）、媒体库入口卡、各库最新入库横排。
- **浏览**：封面墙 / 搜索 / EPUB·PDF 筛选 / 按书名、作者、入库时间、体积排序 / 三档网格密度 / 滚动到底自动续页。
- **详情与下载**：封面、作者、格式、体积、所属库、入库时间、原始文件名；流式下载，支持 Range 断点续传与中文文件名。
- **账号与权限**：管理员 / 普通成员两级角色；首个注册账号自动成为管理员。
- **体验**：深色 / 浅色主题、侧栏抽屉、快捷键 `/` 聚焦搜索、响应式布局。

| 深色 | 浅色 |
|---|---|
| ![深色首页](docs/screenshots/home-dark.webp) | ![浅色首页](docs/screenshots/home-light.webp) |

### 管理员上传媒体库封面

进入「管理媒体库」→ 对应库的「编辑」→「媒体库封面」。支持上传 / 更换 / 恢复自动拼图，建议使用 16:9 横图。

![编辑媒体库封面](docs/screenshots/library-cover.webp)

---

## 快速开始

### 方式 A：Docker Compose（推荐）

```bash
git clone https://github.com/<owner>/libra.git
cd libra

# 首次运行前创建数据目录，并让容器内的 libra 用户（UID 1000）可写
mkdir -p data
sudo chown -R 1000:1000 data

# 修改 docker-compose.yml 里的书籍目录：
#   - /volume1/books:/books:ro  →  你自己的 NAS 路径

docker compose up -d --build
```

打开 `http://<NAS-IP>:8080`。首次访问会引导创建管理员账号 —— **第一个注册的用户自动成为管理员**，即使 `LIBRA_ALLOW_REGISTER=false` 也允许创建首个账号。

### 方式 B：直接使用 GHCR 镜像

GitHub Actions 会构建 `linux/amd64` 与 `linux/arm64` 双架构镜像并推送到 GHCR：

```bash
# Linux / NAS：先准备可写的数据目录
mkdir -p data
sudo chown -R 1000:1000 data

docker run -d \
  --name libra \
  --restart unless-stopped \
  -p 8080:8080 \
  -v "$PWD/data:/data" \
  -v "/path/to/your/books:/books:ro" \
  -e TZ=Asia/Shanghai \
  -e LIBRA_ALLOW_REGISTER=false \
  ghcr.io/<owner>/libra:latest
```

镜像标签：

| 标签 | 来源 |
|---|---|
| `latest` | `main` 分支或 `v*` 标签 |
| `main` | `main` 分支最新构建 |
| `v1.2.3` / `1.2.3` / `1.2` | Git 标签 `v1.2.3` |
| `sha-xxxxxxx` | 对应 commit |

> 把 `<owner>` 换成你的 GitHub 用户名；如果仓库名不是 `libra`，镜像名也要相应替换。
> 首次使用 GHCR 时，包默认可能是私有的。需要匿名拉取的话，到 GitHub 仓库的 Package 设置里把镜像可见性改为 public。

### 方式 C：本地编译

需要 Go 1.22+：

```bash
go mod download
go build -o libra .

LIBRA_DATA_DIR=./data ./libra
# Windows PowerShell:
# $env:LIBRA_DATA_DIR="./data"; .\libra.exe
```

或者：

```bash
make run                   # 编译并本地启动
make linux-amd64           # Linux x86_64
make linux-arm64           # Linux ARM64
make docker                # 本地 docker build
```

前端资源通过 `go:embed` 打进二进制，发布产物只有一个可执行文件。

---

## 首次使用

1. 打开 `http://<NAS-IP>:8080`，注册第一个账号（自动成为管理员）。
2. 进入「管理媒体库」→「新建媒体库」，选择挂载在 `/books` 下的目录。
3. 保存后会立即触发一次扫描；EPUB 会读取内嵌元数据与封面，PDF 会尝试渲染首页封面。
4. 回到首页即可看到媒体库入口卡、精选轮播和各库最新入库。
5. 可选：在「编辑媒体库」里上传自定义封面；不上传则自动拼贴。

PDF 首页渲染依赖容器内的 `poppler-utils`。默认镜像已包含；如果不需要，可在构建时使用 `WITH_POPPLER=0`，PDF 仍可入库和下载，但没有渲染出来的首页封面。

---

## 配置

所有配置都通过环境变量完成：

| 变量 | 默认值 | 说明 |
|---|---|---|
| `LIBRA_ADDR` | `:8080` | 监听地址 |
| `LIBRA_DATA_DIR` | `./data` | 数据根目录（数据库 + 封面 + 图标） |
| `LIBRA_DB_PATH` | `<DATA_DIR>/libra.db` | SQLite 数据库路径 |
| `LIBRA_COVERS_DIR` | `<DATA_DIR>/covers` | 封面目录 |
| `LIBRA_ICONS_DIR` | `<DATA_DIR>/icons` | 上传图标目录 |
| `LIBRA_LIBRARY_ROOT` | 空（不限制） | 允许浏览 / 添加的根目录；容器内通常设为 `/books` |
| `LIBRA_SCAN_CONCURRENCY` | `4` | 扫描时的并行外部进程数（1–32） |
| `LIBRA_COVER_WIDTH` | `600` | PDF 首页渲染宽度（px） |
| `LIBRA_SCAN_ON_START` | `true` | 启动时自动做一次增量扫描 |
| `LIBRA_SESSION_TTL_HOURS` | `720` | 登录会话有效期（小时） |
| `LIBRA_ALLOW_REGISTER` | `true` | 是否允许自助注册；首个账号始终允许创建 |
| `LIBRA_TRUSTED_PROXIES` | 空 | 反向代理地址，用于获取真实客户端 IP |
| `LIBRA_WEB_DIR` | 空 | 开发模式：从磁盘读取 `web/`，修改后刷新即可；生产环境留空 |

### 数据目录

```
<DATA_DIR>/
├── libra.db                  SQLite（WAL 模式）
├── covers/<库ID>/
│   ├── <hash>.jpg            书籍封面
│   └── _library-<时间戳>.jpg 管理员上传的媒体库封面
└── icons/                    管理员上传的图标
```

备份时建议先停止容器再复制整个 `/data`，避免直接拷贝正在写入的 SQLite WAL 文件。

---

## 开发

```bash
# 1. 生成联调样本（可选）
python scripts/gen_samples.py     # 输出 testdata/books：EPUB2 / EPUB3 / PDF
python scripts/gen_demo.py        # 输出 testdata/demo：带封面的示例书库

# 2. 开发模式启动（前端直接读 web/，改完刷新即可）
LIBRA_WEB_DIR=./web go run .
# Windows: $env:LIBRA_WEB_DIR="./web"; go run .

# Windows 一键开发脚本：自动构建、启动、监听前后端改动
powershell -ExecutionPolicy Bypass -File scripts/dev.ps1
```

开发模式使用 `LIBRA_WEB_DIR=web` 时，模板每次请求重新解析、静态资源直接读磁盘；生产环境不设置该变量，使用 `go:embed` 的内嵌资源。

### 目录结构

```
.
├── .github/workflows/
│   └── docker-image.yml       # GitHub Actions：Go 检查 + 多架构镜像构建
├── docs/screenshots/          # README 截图
├── internal/
│   ├── auth/                  # 密码策略、bcrypt、会话令牌
│   ├── config/                # 环境变量
│   ├── httpx/                 # Gin 路由、鉴权、REST handlers
│   ├── scanner/               # 目录遍历、EPUB / PDF 解析、封面生成
│   └── store/                 # SQLite 迁移与 DAO
├── scripts/
│   ├── dev.ps1                # Windows 本地开发启动器
│   ├── gen_demo.py            # 生成带封面的示例书库
│   └── gen_samples.py         # 生成 EPUB / PDF 联调样本
├── web/
│   ├── embed.go               # go:embed 前端资源
│   ├── static/                # app.js / login.js / style.css
│   └── templates/             # app.html / login.html
├── Dockerfile
├── docker-compose.yml
├── Makefile
├── go.mod / go.sum
├── main.go
└── README.md
```

---

## GitHub Actions 与镜像发布

工作流文件：`.github/workflows/docker-image.yml`

触发条件：

- Push 到 `main`
- Push 标签 `v*`（例如 `v1.2.3`）
- Pull Request（只构建，不推送）
- 手动 `workflow_dispatch`

流程：

1. `go vet ./...` + `go build ./...`
2. QEMU + Buildx 构建 `linux/amd64` 和 `linux/arm64`
3. 推送到 `ghcr.io/<owner>/<repo>`
4. 使用 GitHub Actions 缓存加速后续构建

如果推送镜像时报权限错误，检查仓库 **Settings → Actions → General → Workflow permissions**，并确认工作流中的 `packages: write` 权限没有组织策略拦截。

---

## HTTP API 摘要

除 `/healthz`、`/login`、`/api/auth/*` 的部分接口外，其余接口均需要登录。响应统一为：

```json
{ "ok": true, "data": { } }
```

出错时为：

```json
{ "ok": false, "error": "错误信息" }
```

| 权限 | 方法 | 路径 | 说明 |
|---|---|---|---|
| 公开 | GET | `/healthz` | 健康检查 |
| 公开 | GET | `/login` | 登录 / 注册页 |
| 公开 | POST | `/api/auth/login` | 登录 |
| 公开 | POST | `/api/auth/register` | 注册（首个用户成为管理员） |
| 公开 | GET | `/api/auth/me` | 登录状态与初始化状态 |
| 登录 | POST | `/api/auth/logout` | 退出 |
| 登录 | POST | `/api/auth/password` | 修改自己的密码 |
| 登录 | GET | `/api/libraries` | 媒体库列表 |
| 登录 | GET | `/api/libraries/:id` | 单个媒体库 |
| 登录 | GET | `/api/libraries/:id/scan` | 扫描进度快照 |
| 登录 | GET | `/api/books` | 书籍分页列表 |
| 登录 | GET | `/api/featured` | 首页精选轮播 |
| 登录 | GET | `/api/books/:id` | 书籍详情 |
| 登录 | GET | `/download/:id` | 下载原文件 |
| 登录 | GET | `/covers/*` | 书籍封面 / 媒体库封面 |
| 管理员 | POST | `/api/libraries` | 新建媒体库（自动扫描） |
| 管理员 | PUT | `/api/libraries/:id` | 修改名称 / 目录 / 图标 |
| 管理员 | DELETE | `/api/libraries/:id` | 删除媒体库 |
| 管理员 | POST | `/api/libraries/:id/scan` | 触发扫描 |
| 管理员 | POST | `/api/libraries/:id/cover` | 上传媒体库封面 |
| 管理员 | DELETE | `/api/libraries/:id/cover` | 恢复自动拼图 |
| 管理员 | GET | `/api/browse` | 浏览服务器目录 |
| 管理员 | POST | `/api/icons` | 上传图标 |
| 管理员 | GET/POST/PUT/DELETE | `/api/users...` | 账号管理 |

---

## 安全说明

- 密码使用 **bcrypt（cost 12）** 哈希；密码长度至少 8 位，拒绝常见弱口令。
- 会话令牌为 32 字节 CSPRNG 随机值；数据库只存 **SHA-256 摘要**，不存原文。
- Cookie 使用 `HttpOnly` + `SameSite=Lax`；反向代理自动切换 `Secure`。
- 登录失败限流：同一 IP + 用户名 5 分钟内 8 次失败即锁定。
- 所有管理接口由后端 `adminOnly` 中间件拦截，前端隐藏入口只是体验优化。
- 响应头 CSP 只允许同源脚本与样式，无任何外部 CDN 依赖。

---

## 常见问题

**PDF 没有封面？**  
容器内缺少 `poppler-utils`，或 PDF 首页是纯矢量图 / 尺寸不达标。启动日志会提示 `未检测到 pdftoppm`。

**媒体库目录看不到内容？**  
检查 `LIBRA_LIBRARY_ROOT` 与挂载点是否一致。容器默认只允许 `/books` 下的目录。

**上传的媒体库封面在哪里？**  
在 `<DATA_DIR>/covers/<库ID>/_library-*.jpg`。重新扫描或修改目录不会删除它；删除媒体库会连同封面目录一起清理。

**忘记管理员密码？**

```bash
docker exec -it libra /usr/local/bin/libra reset-password admin 新密码
```

**数据会不会丢？**  
只要把 `/data` 挂载到宿主机，删除 / 重建容器不会丢数据。书籍目录以只读方式挂载，Libra 不会修改原始文件。

---

## 说明

- 前端使用原生 HTML / CSS / JavaScript，没有构建步骤，也没有任何外部 CDN 依赖。
- EPUB 解析使用标准库 `archive/zip` + `encoding/xml`，不依赖额外 CGO 库。
- 本项目未附带许可证文件；上传到 GitHub 前请根据你的发布意愿补充 `LICENSE`。
