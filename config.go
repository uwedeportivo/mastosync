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
	Mas          mastodon.Config
	Feeds        []FeedTemplatePair
	NotionToken  string
	NotionParent string
	Bridge       string
	Parent       string
	Mandala      string
}

func InitConfig(path string) error {
	viper.SetConfigPermissions(0600)
	viper.SetConfigFile(path)
	viper.SetDefault("mas", mastodon.Config{
		Server:       "https://some_mastodon_instance",
		ClientID:     "some_client_id",
		ClientSecret: "some_client_secret",
		AccessToken:  "some_access_token",
	})
	viper.SetDefault("feeds", []FeedTemplatePair{{"https://someAFeed.com/xml", "someA.tmpl"},
		{"https://someBFeed.com/xml", "someB.tmpl"}})
	viper.SetDefault("notiontoken", "some_notion_token")
	viper.SetDefault("notionparent", "some_notion_parent_page_id")
	viper.SetDefault("bridge", "some_bridge")
	viper.SetDefault("parent", "some_parent_folder_id")
	viper.SetDefault("mandala", "some_mandala_script")
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
