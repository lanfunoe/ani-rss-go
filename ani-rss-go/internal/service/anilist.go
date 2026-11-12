package service

import (
	"encoding/json"
	"strings"
	"time"

	"ani-rss-go/internal/httpc"
)

const aniListRomajiQuery = "query ($search: String) { Page (page: 1, perPage: 1) { media (search: $search, type: ANIME) { title { romaji native } } } }"

// aniListRomaji 通过 AniList 搜索番剧的罗马音标题
func aniListRomaji(title string) (string, error) {
	if strings.TrimSpace(title) == "" {
		return "", nil
	}
	body, err := json.Marshal(map[string]any{
		"query":     aniListRomajiQuery,
		"variables": map[string]string{"search": title},
	})
	if err != nil {
		return "", err
	}
	resp, err := httpc.Post("https://graphql.anilist.co").
		Set("Content-Type", "application/json").
		Set("Accept", "application/json").
		SetBody(body).
		SetTimeout(5 * time.Second).
		Do()
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := resp.AssertStatus(); err != nil {
		return "", err
	}
	var out struct {
		Data struct {
			Page struct {
				Media []struct {
					Title struct {
						Romaji string `json:"romaji"`
					} `json:"title"`
				} `json:"media"`
			} `json:"Page"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Data.Page.Media) == 0 {
		return "", nil
	}
	return out.Data.Page.Media[0].Title.Romaji, nil
}
