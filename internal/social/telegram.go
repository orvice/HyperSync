package social

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const telegramDefaultAPI = "https://api.telegram.org"

// Telegram Bot API types

type tgResponse struct {
	OK     bool       `json:"ok"`
	Result []tgUpdate `json:"result"`
}

type tgUpdate struct {
	UpdateID    int64      `json:"update_id"`
	ChannelPost *tgMessage `json:"channel_post,omitempty"`
}

type tgMessage struct {
	MessageID       int64         `json:"message_id"`
	Date            int64         `json:"date"`
	Text            string        `json:"text,omitempty"`
	Caption         string        `json:"caption,omitempty"`
	Entities        []tgEntity    `json:"entities,omitempty"`
	CaptionEntities []tgEntity    `json:"caption_entities,omitempty"`
	Chat            *tgChat       `json:"chat,omitempty"`
	Photo           []tgPhotoSize `json:"photo,omitempty"`
	Video           *tgVideo      `json:"video,omitempty"`
	MediaGroupID    string        `json:"media_group_id,omitempty"`
	Sticker         *struct{}     `json:"sticker,omitempty"`
	Document        *struct{}     `json:"document,omitempty"`
	Audio           *struct{}     `json:"audio,omitempty"`
	Poll            *struct{}     `json:"poll,omitempty"`
}

type tgEntity struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
	URL    string `json:"url,omitempty"`
}

type tgPhotoSize struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FileSize     int64  `json:"file_size,omitempty"`
}

type tgVideo struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Duration     int    `json:"duration"`
	FileSize     int64  `json:"file_size,omitempty"`
}

type tgFileResponse struct {
	OK     bool   `json:"ok"`
	Result tgFile `json:"result"`
}

type tgFile struct {
	FileID   string `json:"file_id"`
	FilePath string `json:"file_path"`
}

type tgChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// TelegramClient implements SocialClient for Telegram channel ingestion.
type TelegramClient struct {
	botToken  string
	channelID string
	name      string
	apiBase   string
	client    *http.Client
	cursor    SyncCursorDao
}

// SyncCursorDao persists polling offsets for pull-based content sources.
type SyncCursorDao interface {
	GetOffset(ctx context.Context, platform string) (int64, error)
	SaveOffset(ctx context.Context, platform string, offset int64) error
}

func NewTelegramClient(botToken, channelID, name, apiBase string, cursor SyncCursorDao) *TelegramClient {
	if apiBase == "" {
		apiBase = telegramDefaultAPI
	}
	return &TelegramClient{
		botToken:  botToken,
		channelID: channelID,
		name:      name,
		apiBase:   apiBase,
		client:    &http.Client{Timeout: 30 * time.Second},
		cursor:    cursor,
	}
}

func (t *TelegramClient) Name() string { return t.name }

func (t *TelegramClient) Post(_ context.Context, _ *Post) (interface{}, error) {
	return nil, fmt.Errorf("telegram: posting not implemented")
}

// Compile-time check that TelegramClient satisfies SocialClient.
var _ SocialClient = (*TelegramClient)(nil)

func (t *TelegramClient) ListPosts(ctx context.Context, limit int) ([]*Post, error) {
	var offset int64
	if t.cursor != nil {
		var err error
		offset, err = t.cursor.GetOffset(ctx, t.name)
		if err != nil {
			return nil, fmt.Errorf("telegram: get offset: %w", err)
		}
	}

	url := fmt.Sprintf("%s/bot%s/getUpdates?allowed_updates=[\"channel_post\"]&limit=%d",
		t.apiBase, t.botToken, limit)
	if offset > 0 {
		url += fmt.Sprintf("&offset=%d", offset)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("telegram: create request: %w", err)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telegram: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("telegram: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram: API returned status %d: %s", resp.StatusCode, string(body))
	}

	var tgResp tgResponse
	if err := json.Unmarshal(body, &tgResp); err != nil {
		return nil, fmt.Errorf("telegram: parse response: %w", err)
	}

	if !tgResp.OK {
		return nil, fmt.Errorf("telegram: API returned ok=false")
	}

	// Save offset as max(update_id) + 1 so the next poll skips these updates.
	if t.cursor != nil && len(tgResp.Result) > 0 {
		var maxID int64
		for _, u := range tgResp.Result {
			if u.UpdateID > maxID {
				maxID = u.UpdateID
			}
		}
		if err := t.cursor.SaveOffset(ctx, t.name, maxID+1); err != nil {
			return nil, fmt.Errorf("telegram: save offset: %w", err)
		}
	}

	merged := mergeMediaGroups(tgResp.Result)

	var posts []*Post
	for _, update := range merged {
		msg := update.ChannelPost
		if msg == nil {
			continue
		}

		content := stripEntities(msg.Text, msg.Entities)
		if content == "" {
			content = stripEntities(msg.Caption, msg.CaptionEntities)
		}

		hasPhoto := len(msg.Photo) > 0
		hasVideo := msg.Video != nil

		if content == "" && !hasPhoto && !hasVideo {
			continue
		}

		var media []Media
		if hasPhoto {
			photos := msg.Photo
			// For non-merged messages, Telegram sends multiple sizes of the same
			// photo; take only the largest (last). For merged media groups,
			// mergeMediaGroups already flattened one photo per original message.
			if msg.MediaGroupID == "" && len(photos) > 1 {
				photos = photos[len(photos)-1:]
			}
			for _, photo := range photos {
				fileURL, err := t.getFileURL(ctx, photo.FileID)
				if err != nil {
					return nil, fmt.Errorf("telegram: get photo file: %w", err)
				}
				media = append(media, *NewMediaFromURL(fileURL))
			}
		}
		if hasVideo {
			fileURL, err := t.getFileURL(ctx, msg.Video.FileID)
			if err != nil {
				return nil, fmt.Errorf("telegram: get video file: %w", err)
			}
			media = append(media, *NewMediaFromURL(fileURL))
		}

		posts = append(posts, &Post{
			ID:             strconv.FormatInt(msg.MessageID, 10),
			Content:        content,
			Visibility:     VisibilityLevelPublic,
			Media:          media,
			SourcePlatform: t.name,
			OriginalID:     strconv.FormatInt(msg.MessageID, 10),
			CreatedAt:      time.Unix(msg.Date, 0).UTC(),
		})
	}

	return posts, nil
}

func (t *TelegramClient) getFileURL(ctx context.Context, fileID string) (string, error) {
	url := fmt.Sprintf("%s/bot%s/getFile?file_id=%s", t.apiBase, t.botToken, fileID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var fileResp tgFileResponse
	if err := json.Unmarshal(body, &fileResp); err != nil {
		return "", err
	}

	if !fileResp.OK {
		return "", fmt.Errorf("getFile returned ok=false")
	}

	return fmt.Sprintf("%s/file/bot%s/%s", t.apiBase, t.botToken, fileResp.Result.FilePath), nil
}

// stripEntities processes Telegram entities on a text string.
// Most entities (bold, italic, mention, etc.) are just metadata — the text
// field is already plain. text_link entities carry a hidden URL that would
// be lost, so we insert it after the link text.
func stripEntities(text string, entities []tgEntity) string {
	if len(entities) == 0 {
		return text
	}

	runes := []rune(text)

	// Collect text_link insertions, process from end to avoid offset shifts.
	type insertion struct {
		pos  int
		text string
	}
	var insertions []insertion
	for _, e := range entities {
		if e.Type == "text_link" && e.URL != "" {
			insertions = append(insertions, insertion{
				pos:  e.Offset + e.Length,
				text: " (" + e.URL + ")",
			})
		}
	}

	// Sort by position descending so we can insert without shifting earlier positions.
	for i := 0; i < len(insertions); i++ {
		for j := i + 1; j < len(insertions); j++ {
			if insertions[j].pos > insertions[i].pos {
				insertions[i], insertions[j] = insertions[j], insertions[i]
			}
		}
	}

	for _, ins := range insertions {
		pos := ins.pos
		if pos > len(runes) {
			pos = len(runes)
		}
		expanded := make([]rune, 0, len(runes)+len([]rune(ins.text)))
		expanded = append(expanded, runes[:pos]...)
		expanded = append(expanded, []rune(ins.text)...)
		expanded = append(expanded, runes[pos:]...)
		runes = expanded
	}

	return string(runes)
}

// mergeMediaGroups consolidates updates that share a media_group_id into a
// single update with all photos/videos combined. Caption is taken from the
// first message in the group that has one. Standalone updates pass through
// unchanged. Order: groups appear at the position of their first member;
// standalone updates keep their original position.
func mergeMediaGroups(updates []tgUpdate) []tgUpdate {
	type group struct {
		index int // insertion order
		lead  tgUpdate
	}

	groups := make(map[string]*group)
	var result []tgUpdate
	var order []string // track group insertion order

	for _, u := range updates {
		msg := u.ChannelPost
		if msg == nil || msg.MediaGroupID == "" {
			result = append(result, u)
			continue
		}

		gid := msg.MediaGroupID
		if g, ok := groups[gid]; ok {
			lead := g.lead.ChannelPost
			lead.Photo = append(lead.Photo, msg.Photo...)
			if msg.Video != nil {
				lead.Video = msg.Video
			}
			if lead.Caption == "" && msg.Caption != "" {
				lead.Caption = msg.Caption
			}
			if u.UpdateID > g.lead.UpdateID {
				g.lead.UpdateID = u.UpdateID
			}
		} else {
			clone := u
			cloneMsg := *msg
			cloneMsg.Photo = append([]tgPhotoSize(nil), msg.Photo...)
			clone.ChannelPost = &cloneMsg
			groups[gid] = &group{index: len(result) + len(order), lead: clone}
			order = append(order, gid)
		}
	}

	// Interleave groups into result at their insertion positions.
	if len(order) == 0 {
		return result
	}

	// Rebuild: walk originals and groups in order.
	merged := make([]tgUpdate, 0, len(result)+len(order))
	ri := 0
	for i := 0; i < len(result)+len(order); i++ {
		placed := false
		for _, gid := range order {
			g := groups[gid]
			if g.index == i {
				merged = append(merged, g.lead)
				placed = true
				break
			}
		}
		if !placed {
			if ri < len(result) {
				merged = append(merged, result[ri])
				ri++
			}
		}
	}

	return merged
}
