# Universal Video Proxy (全能视频代理插件)

统一为前端（如 Artplayer）提供任意外部 m3u8 / mp4 资源的无感播放入口，解决跨域、防盗链、重定向、分片多域、Key、Header 注入等常见问题。

## 功能特性
- 单一入口 `/play?url=` 自动识别 & 重写：Master / Media playlist、Variant、分片、Key URI
- 保留全部 variant（后续可扩展筛选）
- 分片统一 `/seg?u=...`
- Key 统一 `/key?u=...`
- 原始直通 `/raw?url=...`（调试）
- 签名功能可选（当前默认关闭；开启后可防他人盗用）
- Header 注入（UA / Referer / Cookie / Host 重写）
- 域名白名单
- Range 支持（mp4 / ts）
- m3u8 与 key 内存缓存（LRU+TTL）；ts 缓存默认关闭
- CORS 支持
- Playlist 大小 / URL 长度限制

## 快速使用
```bash
cd plugins/universal-video-proxy
cp config.example.json config.json
# 编辑 config.json: allowHosts / (可选) 启用签名
bash run.sh
```
默认监听`:18200`

健康检查：
```
curl http://127.0.0.1:18200/health
```

### 播放示例（未启用签名）
前端直接：
```
/play?url=
```

### 启用签名（可选）
1. 将 `config.json` 中 `sign.enabled` 改为 `true`，设置强随机 `secret`。
2. 重新启动。
3. 先调 `/sign?raw=<真实URL>` 获取 JSON：`{ proxy, sign, ts }`
4. 使用返回的 `proxy` 作为播放器 URL。

### Artplayer 示例
```js
async function play(realUrl){
  // 如果未启用签名：
  const url = '/play?url=' + encodeURIComponent(realUrl);
  new Artplayer({ container:'#player', url, type: url.includes('.m3u8') ? 'm3u8' : 'auto' });
}

async function playWithSign(realUrl){
  const r = await fetch('/sign?raw=' + encodeURIComponent(realUrl));
  const data = await r.json();
  new Artplayer({ container:'#player', url: data.proxy, type: data.proxy.includes('.m3u8') ? 'm3u8':'auto' });
}
```

## 主要接口
| 路径 | 描述 |
|------|------|
| /play | 主播放入口（m3u8/mp4 自动） |
| /seg | 分片（ts, fmp4 等） |
| /key | HLS AES-128 Key |
| /raw | 不重写直通调试 |
| /sign | 生成签名（启用后有效） |
| /health | 健康检查 |

## 签名机制（可选）
```
sign = HEX( HMAC_SHA256(url + "|" + ts, secret) )
窗口 = ttlSeconds (默认 600 )
```
请求需包含：`url` / `sign` / `ts`。分片与 key 会继承初次 /play 的 sign+ts。

## 配置说明
见 `config.example.json` 注释；关键字段：
- `allowHosts`: 上游允许的域名白名单
- `sign.enabled`: 是否启用签名（默认 false）
- `cache.ts.enabled`: 仍默认 false，避免内存占用

## 安全建议
1. 生产环境建议启用签名 + 强随机 secret。
2. 精确限制 `allowHosts`。
3. 可前置 Nginx 做速率限制。

## 常见问题
| 问题 | 原因 | 解决 |
|------|------|------|
| 401 (签名) | 启用签名但参数缺失/过期 | 重新获取 /sign |
| 403 | 不在 allowHosts | 加入域名或确认 URL 正确 |
| Range 不生效 | 上游不支持 Range | 目前仅透传，不做模拟 |
| 播放列表过大 | 超出 maxPlaylistKB | 提升该配置或确认 URL 正确 |

## 后续扩展可能
- 变体筛选（最高/指定分辨率）
- DASH 支持
- 上游 socks/http 代理
- 分片预取 / ts 缓存策略

## 许可证
MIT（仅本插件代码）。确保对被代理资源拥有合法访问权。