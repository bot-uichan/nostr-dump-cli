package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/btcsuite/btcutil/bech32"
	"github.com/gorilla/websocket"
)

type Event struct {
	ID        string `json:"id"`
	PubKey    string `json:"pubkey"`
	CreatedAt int64  `json:"created_at"`
	Kind      int    `json:"kind"`
	Content   string `json:"content"`
}

type Filter struct {
	Authors []string `json:"authors,omitempty"`
	Kinds   []int    `json:"kinds,omitempty"`
	Limit   int      `json:"limit,omitempty"`
	Since   int64    `json:"since,omitempty"`
	Until   int64    `json:"until,omitempty"`
}

func main() {
	var (
		npub       = flag.String("npub", "", "target npub (required)")
		relaysRaw  = flag.String("relays", "wss://relay.damus.io,wss://nos.lol,wss://relay.nostr.band", "comma-separated relay URLs")
		kindsRaw   = flag.String("kinds", "1", "comma-separated kinds")
		batch      = flag.Int("batch", 500, "events per relay per page")
		out        = flag.String("out", "posts.jsonl", "output JSONL file")
		timeoutSec = flag.Int("timeout", 15, "relay read timeout seconds")
		since      = flag.Int64("since", 0, "lower bound created_at (unix seconds)")
		until      = flag.Int64("until", 0, "upper bound created_at (unix seconds)")
		maxPages   = flag.Int("max-pages", 0, "stop after N pages (0 = unlimited)")
	)
	flag.Parse()

	if *npub == "" {
		log.Fatal("--npub is required")
	}
	if *batch <= 0 {
		log.Fatal("--batch must be > 0")
	}

	pubkey, err := npubToHex(*npub)
	if err != nil {
		log.Fatalf("invalid npub: %v", err)
	}

	relays := splitTrim(*relaysRaw)
	if len(relays) == 0 {
		log.Fatal("no relays provided")
	}
	kinds, err := parseKinds(*kindsRaw)
	if err != nil {
		log.Fatalf("invalid --kinds: %v", err)
	}

	seen := make(map[string]struct{}, 20000)
	all := make([]Event, 0, 20000)

	cursorUntil := *until
	page := 0

	for {
		if *maxPages > 0 && page >= *maxPages {
			break
		}
		page++

		pageEvents := make([]Event, 0, len(relays)*(*batch))
		oldest := int64(1<<62 - 1)

		for _, relay := range relays {
			filter := Filter{
				Authors: []string{pubkey},
				Kinds:   kinds,
				Limit:   *batch,
			}
			if *since > 0 {
				filter.Since = *since
			}
			if cursorUntil > 0 {
				filter.Until = cursorUntil
			}

			events, ferr := fetchRelayPage(relay, filter, time.Duration(*timeoutSec)*time.Second)
			if ferr != nil {
				log.Printf("[warn] relay=%s page=%d err=%v", relay, page, ferr)
				continue
			}
			for _, ev := range events {
				if _, ok := seen[ev.ID]; ok {
					continue
				}
				seen[ev.ID] = struct{}{}
				pageEvents = append(pageEvents, ev)
				if ev.CreatedAt < oldest {
					oldest = ev.CreatedAt
				}
			}
		}

		if len(pageEvents) == 0 {
			log.Printf("done: no more events at page=%d", page)
			break
		}

		all = append(all, pageEvents...)
		log.Printf("page=%d got=%d total=%d until=%d", page, len(pageEvents), len(all), cursorUntil)

		if oldest <= 0 || oldest == int64(1<<62-1) {
			log.Printf("done: no valid oldest timestamp at page=%d", page)
			break
		}
		next := oldest - 1
		if cursorUntil > 0 && next >= cursorUntil {
			log.Printf("done: pagination cursor stopped moving (old=%d new=%d)", cursorUntil, next)
			break
		}
		if *since > 0 && next < *since {
			cursorUntil = *since
			log.Printf("reached since boundary: %d", *since)
			break
		}
		cursorUntil = next
	}

	sort.Slice(all, func(i, j int) bool {
		if all[i].CreatedAt == all[j].CreatedAt {
			return all[i].ID < all[j].ID
		}
		return all[i].CreatedAt < all[j].CreatedAt
	})

	if err := writeJSONL(*out, all); err != nil {
		log.Fatalf("failed to write output: %v", err)
	}
	log.Printf("done: wrote %d events to %s", len(all), *out)
}

func fetchRelayPage(relayURL string, filter Filter, timeout time.Duration) ([]Event, error) {
	conn, _, err := websocket.DefaultDialer.Dial(relayURL, nil)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	subID := randomSubID()
	req := []any{"REQ", subID, filter}
	if err := conn.WriteJSON(req); err != nil {
		return nil, err
	}
	defer func() {
		_ = conn.WriteJSON([]any{"CLOSE", subID})
	}()

	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}

	events := make([]Event, 0, filter.Limit)
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return events, err
		}

		var raw []json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil || len(raw) < 2 {
			continue
		}

		var typ string
		if err := json.Unmarshal(raw[0], &typ); err != nil {
			continue
		}

		switch typ {
		case "EVENT":
			if len(raw) < 3 {
				continue
			}
			var gotSubID string
			if err := json.Unmarshal(raw[1], &gotSubID); err != nil || gotSubID != subID {
				continue
			}
			var ev Event
			if err := json.Unmarshal(raw[2], &ev); err != nil {
				continue
			}
			events = append(events, ev)
		case "EOSE":
			var gotSubID string
			if len(raw) >= 2 && json.Unmarshal(raw[1], &gotSubID) == nil && gotSubID == subID {
				return events, nil
			}
		}
	}
}

func writeJSONL(path string, events []Event) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, ev := range events {
		b, err := json.Marshal(ev)
		if err != nil {
			return err
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
		if err := w.WriteByte('\n'); err != nil {
			return err
		}
	}
	return w.Flush()
}

func npubToHex(npub string) (string, error) {
	hrp, data, err := bech32.Decode(npub)
	if err != nil {
		return "", err
	}
	if hrp != "npub" {
		return "", errors.New("bech32 hrp is not npub")
	}
	b, err := bech32.ConvertBits(data, 5, 8, false)
	if err != nil {
		return "", err
	}
	if len(b) != 32 {
		return "", fmt.Errorf("unexpected pubkey length: %d", len(b))
	}
	return hex.EncodeToString(b), nil
}

func splitTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseKinds(s string) ([]int, error) {
	parts := splitTrim(s)
	if len(parts) == 0 {
		return nil, errors.New("empty kinds")
	}
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func randomSubID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("sub-%d", time.Now().UnixNano())
	}
	return "sub-" + hex.EncodeToString(b)
}
