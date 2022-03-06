package twitterscraper

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	twv1 "github.com/dghubble/go-twitter/twitter"
)

const twitterBearerToken = "AAAAAAAAAAAAAAAAAAAAAPYXBAAAAAAACLXUNDekMxqa8h%2F40K4moUkGsoc%3DTYfbDKbT3jJPCEVnMYqilB28NHfOPqkca3qaAxGfsyKCs0wRbw"

var (
	ErrTwitterNotFound  = errors.New("tweet not found")
	ErrTwitterRateLimit = errors.New("twitter rate limit hit")
)

type ScrapedTweet struct {
	twv1.Tweet

	UserID         uint64 `json:"user_id"`
	ConversationID uint64 `json:"conversation_id"`
}

type ScrapeResponse struct {
	Tweets map[string]*ScrapedTweet `json:"tweets"`
	Users  map[string]*twv1.User    `json:"users"`
	Error  string                   `json:"error"`
}

type twitterResponse struct {
	GlobalObjects struct {
		Tweets map[string]*ScrapedTweet `json:"tweets"`
		Users  map[string]*twv1.User    `json:"users"`
	} `json:"globalObjects"`
}

type Scraper struct {
	guestToken       string
	guestTokenCreate time.Time
	l                sync.Mutex
	client           *http.Client
}

func NewScraper() *Scraper {
	return &Scraper{
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
		l:                sync.Mutex{},
		guestToken:       "",
		guestTokenCreate: time.Now(),
	}
}

func (s *Scraper) getGuestToken() error {
	req, err := http.NewRequest("POST", "https://api.twitter.com/1.1/guest/activate.json", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+twitterBearerToken)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		content, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("response status %s: %s", resp.Status, content)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var jsn map[string]interface{}
	if err := json.Unmarshal(body, &jsn); err != nil {
		return err
	}
	var ok bool
	if s.guestToken, ok = jsn["guest_token"].(string); !ok {
		return fmt.Errorf("guest_token not found")
	}
	s.guestTokenCreate = time.Now()

	return nil
}

func (s *Scraper) apiRequest(req *http.Request, target *twitterResponse) error {
	s.l.Lock()
	defer s.l.Unlock()

	// add sleep here if necessary

	if s.guestToken == "" || time.Since(s.guestTokenCreate) >= 2*time.Hour {
		if err := s.getGuestToken(); err != nil {
			return err
		}
	}

	req.Header.Set("Authorization", "Bearer "+twitterBearerToken)
	req.Header.Set("X-Guest-Token", s.guestToken)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		target = nil
		return ErrTwitterNotFound
	}

	if resp.StatusCode == 429 {
		s.guestToken = ""
		return ErrTwitterRateLimit
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusForbidden {
		content, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("twitter response status %s: %s", resp.Status, content)
	}

	if resp.Header.Get("X-Rate-Limit-Remaining") == "0" {
		s.guestToken = ""
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

func (s *Scraper) newRequest(method string, url string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("include_profile_interstitial_type", "1")
	q.Add("include_blocking", "1")
	q.Add("include_blocked_by", "1")
	q.Add("include_followed_by", "1")
	q.Add("include_want_retweets", "1")
	q.Add("include_mute_edge", "1")
	q.Add("include_can_dm", "1")
	q.Add("include_can_media_tag", "1")
	q.Add("include_ext_has_nft_avatar", "1")
	q.Add("skip_status", "1")
	q.Add("cards_platform", "Web-12")
	q.Add("include_cards", "1")
	q.Add("include_ext_alt_text", "true")
	q.Add("include_quote_count", "true")
	q.Add("include_reply_count", "1")
	q.Add("tweet_mode", "extended")
	q.Add("include_entities", "true")
	q.Add("include_user_entities", "true")
	q.Add("include_ext_media_color", "true")
	q.Add("include_ext_media_availability", "true")
	q.Add("include_ext_sensitive_media_warning", "true")
	q.Add("send_error_codes", "true")
	q.Add("simple_quoted_tweet", "true")
	q.Add("include_tweet_replies", "true")
	q.Add("ext", "mediaStats,highlightedLabel,hasNftAvatar,voiceInfo,superFollowMetadata")
	req.URL.RawQuery = q.Encode()

	return req, nil
}

func (s *Scraper) GetAllFromTweet(id uint64) (map[string]*ScrapedTweet, map[string]*twv1.User, error) {
	req, err := s.newRequest("GET", fmt.Sprintf("https://twitter.com/i/api/2/timeline/conversation/%v.json", id))
	if err != nil {
		return nil, nil, err
	}

	var resp twitterResponse

	if err := s.apiRequest(req, &resp); err != nil {
		return nil, nil, err
	}

	return resp.GlobalObjects.Tweets, resp.GlobalObjects.Users, nil
}
