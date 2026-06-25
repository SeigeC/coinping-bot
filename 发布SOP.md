# CoinPing 发布 SOP：Dev.to → HN → Medium 三连发

> 用这篇指南，三篇文章能在 30 分钟内发完。按顺序执行，效果叠加。

## 前置准备

- [ ] dev.to 已登录
- [ ] HN 已登录
- [ ] Medium 已登录（可选）
- [ ] 获取 dev.to API key：dev.to → Settings → Extensions → DEV Community API Keys → Generate

---

## 步骤 1：发布到 Dev.to

**方式 A：API 自动发布（最快）**

```bash
# 设置你的 dev.to API key
export DEVTO_KEY="你的key"

# 用 Python 构建 payload 并发布
python3 -c "
import json, os, subprocess

with open('/Users/c/.neo/workspaces/ws-39icbaip/双线启动/bot/devto-post-building-coinping.md') as f:
    content = f.read()

# 标题在文件第一行 (去开头的 #)
lines = content.split('\n')
title = lines[0].replace('# ', '').strip()

# 正文去掉标题行和前面的元信息行，到正文开始的 '---' 之后
# 找到第一个 --- 之后的内容
parts = content.split('---', 2)
body = parts[2].strip() if len(parts) > 2 else '\n'.join(lines[4:])

payload = json.dumps({
    'article': {
        'title': title,
        'body_markdown': body,
        'published': False,  # 先发草稿，确认后再发布
        'tags': ['go', 'crypto', 'telegram', 'webdev', 'tutorial']
    }
})

with open('/tmp/devto-payload.json', 'w') as f:
    f.write(payload)

import subprocess
result = subprocess.run([
    'curl', '-s', '-w', '\nHTTP:%{http_code}',
    '-X', 'POST', 'https://dev.to/api/articles',
    '-H', f'api-key: {os.environ[\"DEVTO_KEY\"]}',
    '-H', 'Content-Type: application/json',
    '--data', f'@/tmp/devto-payload.json'
], capture_output=True, text=True)
print(result.stdout)
"
```

返回 `HTTP:201` 即成功。会得到文章 URL。

**方式 B：手动粘贴（如果 API Key 不顺手）**

1. 打开 https://dev.to/new
2. 标题粘贴：`Building a Zero-Budget Crypto Alert Bot with Go, SQLite, and Telegram`
3. 正文从 `devto-post-building-coinping.md` 复制（从 "I couldn't find a crypto alert bot..." 开始）
4. Tags 加：`go`, `crypto`, `telegram`, `webdev`, `tutorial`
5. 点 "Save Draft" → 预览 → "Publish"

---

## 步骤 2：发布到 Hacker News

**文件**：`双线启动/bot/show-hn-文案.md`

1. 打开 https://news.ycombinator.com/submit
2. Title：`Show HN: CoinPing – Multi-condition crypto alerts on Telegram, built in Go`
3. URL：填 **dev.to 文章链接**（不是 bot 链接，因为 Show HN 应指向你写的文章/bot 主页，dev.to 链接更好——可以引流回去）
   - 如果想直接指向 bot：`https://t.me/CoinPingAlertBot`
   - 如果想指向落地页：`https://seigec.github.io/coinping-bot/`
4. Text（可选，如果 URL 指向 bot）：粘贴 show-hn-文案.md 的正文
5. 按 "Submit"

> 最佳发布时间：北京时间 周二/周三 晚上 10:00 PM（美东 10:00 AM，HN 流量巅峰）

---

## 步骤 3（可选）：导入 Medium

1. 打开 https://medium.com/p/import
2. 粘贴你的 dev.to 文章 URL
3. Medium 自动导入 + 设置 canonical URL 指向 dev.to（SEO 利好）

---

## 步骤 4：发 Twitter 推广

**文件**：`devto-post-building-coinping.md` 末尾 "## Social Media Versions" 部分

3 条 thread 已经写好，字数都校验过不超过 280。直接在 Twitter 发 thread，第一条后回复第二条，第二条后回复第三条。

---

## 步骤 5：发中文社区

**文件**：`双线启动/bot/中文推广-币圈文章.md`

- **知乎**：直接发全文
- **币乎**：直接发全文
- **微信群/Telegram 群**：用文末的话术版本，自然引出 bot

---

## 发布后 Check

- [ ] dev.to 文章能打开
- [ ] HN 帖子可见（检查 new 页面 /shownew）
- [ ] 回复 dev.to 和 HN 的前几条评论（前 2 小时最重要）
- [ ] 24h 后看 dev.to 和 HN 数据，决定要不要追加推广
