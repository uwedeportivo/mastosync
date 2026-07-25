package main

import (
	"bytes"
	"context"
	"html"
	"net/url"
	"text/template"
	"time"

	appbsky "github.com/bluesky-social/indigo/api/bsky"
	skybot "github.com/danrusei/gobot-bsky"
	mdon "github.com/mattn/go-mastodon"
	"github.com/mmcdole/gofeed"
)

const kMastodonMaxTootLen = 500
const kBlueskyMaxTootLen = 300

type Poster interface {
	Post(item *gofeed.Item, tmpl *template.Template) (string, error)
}

type MastodonPoster struct {
	mClient *mdon.Client
}

func (mpr *MastodonPoster) Post(item *gofeed.Item, tmpl *template.Template) (string, error) {
	item.Description = stripTagsPolicy.Sanitize(item.Description)
	buf := new(bytes.Buffer)
	err := tmpl.Execute(buf, item)
	if err != nil {
		return "", err
	}
	tootStr := buf.String()
	if len(tootStr) > kMastodonMaxTootLen {
		tootStr = tootStr[:kMastodonMaxTootLen]
	}
	toot := mdon.Toot{
		Status: html.UnescapeString(tootStr),
	}

	status, err := mpr.mClient.PostStatus(context.Background(), &toot)
	if err != nil {
		return "", err
	}

	return string(status.ID), nil
}

type BlueskyPoster struct {
	skyAgent *skybot.BskyAgent
}

func (bpr *BlueskyPoster) Post(item *gofeed.Item, tmpl *template.Template) (string, error) {
	u, err := url.Parse(item.Link)
	if err != nil {
		return "", err
	}
	item.Description = stripTagsPolicy.Sanitize(item.Description)
	buf := new(bytes.Buffer)
	err = tmpl.Execute(buf, item)
	if err != nil {
		return "", err
	}
	tootStr := buf.String()
	if len(tootStr) > kBlueskyMaxTootLen {
		tootStr = tootStr[:kBlueskyMaxTootLen]
	}

	post := appbsky.FeedPost{
		LexiconTypeID: "app.bsky.feed.post",
		Text:          html.UnescapeString(tootStr),
		CreatedAt:     time.Now().Format(time.RFC3339),
		Embed: &appbsky.FeedPost_Embed{
			EmbedExternal: &appbsky.EmbedExternal{
				LexiconTypeID: "app.bsky.embed.external",
				External: &appbsky.EmbedExternal_External{
					Title:       html.UnescapeString(item.Title),
					Uri:         u.String(),
					Description: html.UnescapeString(item.Title),
				},
			},
		},
	}

	ctx := context.Background()
	cid, _, err := bpr.skyAgent.PostToFeed(ctx, post)
	if err != nil {
		return "", err
	}
	return cid, nil
}
