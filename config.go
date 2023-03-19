package main

import (
	"github.com/mattn/go-mastodon"
	"github.com/spf13/viper"
)

type FeedTemplatePair struct {
	FeedURL  string
	Template string
}

type Config struct {
	Mas   mastodon.Config
	Feeds []FeedTemplatePair
}

func InitConfig(path string) error {
	viper.SetConfigPermissions(0600)
	viper.SetConfigFile(path)
	viper.SetDefault("mas", mastodon.Config{
		Server:       "https://mastodon.social",
		ClientID:     "some_client_id",
		ClientSecret: "some_client_secret",
		AccessToken:  "some_access_token",
	})
	viper.SetDefault("feeds", []FeedTemplatePair{{"http://some_RSS_feed.com/xml", "path_to_some.tmpl"},
		{"http://some_other_RSS_feed.com/xml", "path_to_some_other.tmpl"}})
	return viper.WriteConfig()
}

func ReadConfig(path string) (*Config, error) {
	viper.SetConfigFile(path)
	err := viper.ReadInConfig()
	if err != nil {
		return nil, err
	}
	c := &Config{}
	err = viper.Unmarshal(c)
	if err != nil {
		return nil, err
	}
	return c, nil
}
