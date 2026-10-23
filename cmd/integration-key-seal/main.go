// Command integration-key-seal rewrites one company's integration key in Redis
// into its sealed form, or back to plain (DEV-3295 QA helper, dev only).
//
// It reads the current value at {company_id}:setting:{key}, seals (or opens)
// it with INTEGRATION_KEYS_SECRET, and writes it back in the same JSON-string
// shape tms-auth uses. The key itself is never printed.
//
//	REDIS_URL=redis://:pass@host:6379 INTEGRATION_KEYS_SECRET=... \
//	  go run ./cmd/integration-key-seal -company <uuid> -key here_api_key
//	... -restore    # back to plain
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/TMS360/backend-pkg/keycrypt"
)

func main() {
	companyID := flag.String("company", "", "company id")
	settingKey := flag.String("key", "", "integration setting key, e.g. here_api_key")
	restore := flag.Bool("restore", false, "write the plain value back")
	flag.Parse()

	if *companyID == "" || *settingKey == "" {
		log.Fatal("both -company and -key are required")
	}
	if err := keycrypt.LoadFromEnv(); err != nil {
		log.Fatal(err)
	}
	opts, err := redis.ParseURL(os.Getenv("REDIS_URL"))
	if err != nil {
		log.Fatalf("REDIS_URL: %v", err)
	}
	rdb := redis.NewClient(opts)
	defer rdb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := rewrite(ctx, rdb, *companyID, *settingKey, *restore); err != nil {
		log.Fatal(err)
	}
}

func rewrite(ctx context.Context, rdb *redis.Client, companyID, settingKey string, restore bool) error {
	redisKey := fmt.Sprintf("%s:setting:%s", companyID, settingKey)

	raw, err := rdb.Get(ctx, redisKey).Bytes()
	if err != nil {
		return fmt.Errorf("read %s: %w", redisKey, err)
	}
	var stored string
	if err := json.Unmarshal(raw, &stored); err != nil {
		return fmt.Errorf("%s is not a JSON string: %w", redisKey, err)
	}

	next, err := transform(companyID, settingKey, stored, restore)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return err
	}
	// KeepTTL: tms-auth writes settings without expiry; do not invent one.
	if err := rdb.Set(ctx, redisKey, encoded, redis.KeepTTL).Err(); err != nil {
		return fmt.Errorf("write %s: %w", redisKey, err)
	}
	log.Printf("%s is now %s", redisKey, describe(next))
	return nil
}

func transform(companyID, settingKey, stored string, restore bool) (string, error) {
	if restore {
		return keycrypt.OpenSetting(companyID, settingKey, stored)
	}
	return keycrypt.SealSetting(companyID, settingKey, stored)
}

func describe(value string) string {
	if keycrypt.IsSealed(value) {
		return "sealed"
	}
	return "plain"
}
