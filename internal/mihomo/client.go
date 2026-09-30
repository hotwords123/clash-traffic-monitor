package mihomo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/coder/websocket"
)

type Metadata struct {
	Network         string `json:"network"`
	Type            string `json:"type"`
	SourceIP        string `json:"sourceIP"`
	DestinationIP   string `json:"destinationIP"`
	SourcePort      string `json:"sourcePort"`
	DestinationPort string `json:"destinationPort"`
	Host            string `json:"host"`
	SniffHost       string `json:"sniffHost"`
	Process         string `json:"process"`
	ProcessPath     string `json:"processPath"`
}

type Connection struct {
	ID          string   `json:"id"`
	Metadata    Metadata `json:"metadata"`
	Upload      int64    `json:"upload"`
	Download    int64    `json:"download"`
	Start       string   `json:"start"`
	Chains      []string `json:"chains"`
	Rule        string   `json:"rule"`
	RulePayload string   `json:"rulePayload"`
}

type Snapshot struct {
	DownloadTotal int64        `json:"downloadTotal"`
	UploadTotal   int64        `json:"uploadTotal"`
	Connections   []Connection `json:"connections"`
}

type Client struct {
	baseURL  string
	secret   string
	interval int
}

func New(baseURL, secret string, intervalMS int) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), secret: secret, interval: intervalMS}
}

func (c *Client) Stream(ctx context.Context, receive func(Snapshot) error) error {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("parse mihomo URL: %w", err)
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		return fmt.Errorf("unsupported mihomo URL scheme %q", u.Scheme)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/connections"
	query := u.Query()
	query.Set("interval", fmt.Sprintf("%d", c.interval))
	u.RawQuery = query.Encode()

	header := http.Header{}
	if c.secret != "" {
		header.Set("Authorization", "Bearer "+c.secret)
	}
	conn, _, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		return fmt.Errorf("connect to mihomo: %w", err)
	}
	defer conn.CloseNow()
	conn.SetReadLimit(16 << 20)

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return fmt.Errorf("read mihomo snapshot: %w", err)
		}
		var snapshot Snapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			return fmt.Errorf("decode mihomo snapshot: %w", err)
		}
		if err := receive(snapshot); err != nil {
			return err
		}
	}
}
