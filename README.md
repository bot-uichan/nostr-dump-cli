# nostr-dump-cli

`npub` を指定して、Relay から投稿をできるだけ全件取得して JSONL を標準出力する CLI です。

## Install

```bash
go mod tidy
go build -o nostr-dump .
```

## Usage

```bash
./nostr-dump \
  --npub npub1xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx \
  --relays wss://relay.damus.io,wss://nos.lol,wss://relay.nostr.band \
  --kinds 1 \
  --batch 500 > posts.jsonl
```

### 主なオプション

- `--npub` (required): 対象ユーザー
- `--relays`: カンマ区切りの Relay URL
- `--kinds`: 取得対象 kind（例: `1,6`）
- `--batch`: 1 relay あたり 1ページで取る件数
- `--since`: created_at 下限 (unix sec)
- `--until`: created_at 上限 (unix sec)
- `--max-pages`: ページ上限（0で無制限）

※ JSONLは常に stdout に出ます。進捗ログは stderr に出ます。

## ページネーション戦略

1. `REQ` を `limit + until` 付きで送信
2. `EOSE` まで `EVENT` を収集
3. 最古 `created_at` を見て次の `until = oldest - 1`
4. これをイベントが尽きるまで繰り返す

## 注意

- Nostr には「全履歴を絶対返す」保証はありません。各 Relay に保持されている範囲が上限です。
- Relay を複数使うことで取りこぼしに強くなります。
- 重複は `event.id` で除去しています。
