package conf

var (
	Conf *Config
)

type Config struct {
	Socials map[string]*SocialConfig
}

func (c *Config) Print() {}

type SocialConfig struct {
	Type     string
	Enabled  bool
	Mastodon *MastodonConfig
	Bluesky  *BlueskyConfig
}

type MastodonConfig struct {
	Instance string
	Token    string
}

type BlueskyConfig struct {
	Host     string
	Handle   string
	Password string
}
