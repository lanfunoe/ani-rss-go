<div align="center">
<h1 align="center">ANI-RSS-GO</h1>
<p align="center">
<strong>基于 RSS 自动追番、订阅、下载、刮削、洗版</strong>
</p>

[ani-rss](https://github.com/wushuo894/ani-rss) 的 Go 后端实现，前端沿用 ani-rss-ui

![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)
![License](https://img.shields.io/badge/license-GPL--2.0-blue)
![Platform](https://img.shields.io/badge/platform-linux-lightgrey?logo=linux&logoColor=white)

</div>


## 简介

后端参照 ani-rss Java 版（3.2.59）源码，用 Go 重新实现；前端 `ani-rss-ui` 来自上游并内嵌进二进制。
单个静态二进制即可运行，无需 JRE，内存与磁盘占用远低于 Java 版。

配置文件沿用 Java 版格式（`config.v2.json`、`ani.v2.json`），使用方式与 [ani-rss 文档](https://docs.wushuo.top) 一致。

## 功能

- **订阅**：Mikan、AnimeGarden、AniBT 番剧浏览与 RSS 订阅，备用 RSS，合集下载
- **下载器**：qBittorrent、Transmission、Aria2
- **整理**：自动重命名、集数偏移、洗版、TMDB / Bangumi / AniList 元数据刮削
- **通知**：Telegram、邮件、Server 酱、Bark、WebHook、AstrBot、Shell、系统通知、Emby 媒体库刷新、文件移动、OpenList 上传
- **联动**：Bangumi 授权与收藏同步，Emby WebHook 播放后自动点格子
- **其他**：ICS 追番日历、在线播放、配置备份与恢复、IP 白名单

## 与 Java 版（3.2.59）资源占用对比

### 磁盘占用

| 项目  | Go       | Java 3.2.59               |
|-----|----------|---------------------------|
| 程序  | 41.6 MiB | 58.1 MiB（ani-rss.jar）     |
| 运行时 | 无        | 190.8 MiB（Temurin 25 JRE） |
| 合计  | 41.6 MiB | 248.9 MiB                 |

### 空配置

| 项目          | Go            | Java 3.2.59     |
|-------------|---------------|-----------------|
| 内存占用（RSS）   | 28.2 ~ 43 MiB | 232.5 ~ 658 MiB |
| 启动 CPU 时间   | < 0.01 s      | 3.69 s          |
| CPU 占用（平均）  | 0.003%        | 0.063%          |

### 技术栈

- Go 1.27、Gin、标准库 HTTP 客户端
- `log/slog`、lumberjack：内存日志列表与滚动日志文件
- goquery、go-pinyin：HTML 解析与拼音处理
- golang-ical、x/mod/semver：ICS 日历、版本比较
- golang-jwt：HS256 登录令牌
- goldmark、`html/template`：HTML 邮件通知
- robfig/cron：定时任务
- `encoding/json`：配置、订阅及重命名缓存，写入时先写临时文件再原子替换；后台任务使用 goroutine 与定时间隔循环
- ttlcache：带过期时间的进程内缓存

## TODO

1. 非 Linux 平台支持（Windows、macOS）与开机自启
2. MCP 服务
3. 程序自动更新
4. cicd
5. docker 构建
6. 写 bug
7. 把 ai 写的代码捏成我想要的形状
8. 再捏一个图标
9. 镑十五便士

## 致谢

- [wushuo894/ani-rss](https://github.com/wushuo894/ani-rss)：原项目，本项目的前端与功能设计均来自于此

### ani-rss 相关文章

- [猫猫博客 Docker 部署 ani-rss 实现自动追番](https://catcat.blog/docker-ani-rss.html)
- [从零开始的NAS生活 第四回：ANI-RSS，自动追番！](https://www.wtsss.fun/archives/qhaQ3M7v)
- [自动化追番计划](http://jinghuashang.cn/posts/8f622332.html)
- [ANI-RSS：自动追番新姿势！](https://www.himiku.com/archives/ani-rss.html)
- [📺 彻底解放双手！2025年最新 Ani-RSS + qBittorrent 全自动追番保姆级教程](http://www.nuan1145.eu.cc/archives/wei-ming-ming-wen-zhang-75sNBWk0)

## 许可证
基于 [GPL-2.0](LICENSE) 发布。

## 免责声明

### 项目性质

本工具为中立性技术辅助工具，通过自动化程序抓取互联网公开分享的种子文件链接 (非存储内容)，并向用户指定的下载工具 (如
qBittorrent、Transmission、Aria2 等)推送任务指令。工具本身不具备资源存储、分发及内容审查功能

### 用户责任

- 合法性承诺：用户需确保下载行为及文件使用符合所在国家/地区的《著作权法》《网络安全法》等法规，禁止用于盗版、非法传播等用途
- 自担风险：种子文件的合法性、安全性（如病毒、违规内容）由资源提供方独立负责，用户需自行验证并承担由此引发的法律与经济风险

### 开发者免责

- 技术中立性：开发者仅维护工具的功能实现，不参与种子文件的内容控制、编辑或优化，亦无法保证链接有效性、完整性与获取速率
- 免责范围
    - 用户因使用第三方种子导致的设备损害、数据丢失或法律纠纷
    - 因网络政策、技术更新或源站限制造成的服务中断或功能失效
- 例外追责
    - 若监管机构认定本工具违背技术中立原则，开发者保留终止服务的权利
