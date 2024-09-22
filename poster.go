package main

import (
	"bytes"
	"context"
	"net/url"
	"text/template"

	skybot "github.com/danrusei/gobot-bsky"
	mdon "github.com/mattn/go-mastodon"
	"github.com/mmcdole/gofeed"
)

const kMastodonMaxTootLen = 500
const kBlueskyMaxTootLen = 300

type Poster interface {
	Post(item *gofeed.Item, tmpl *template.Template, tags []string) (string, error)
}

type MastodonPoster struct {
	mClient *mdon.Client
}

func (mpr *MastodonPoster) Post(item *gofeed.Item, tmpl *template.Template, tags []string) (string, error) {
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
		Status: tootStr,
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

func (bpr *BlueskyPoster) Post(item *gofeed.Item, tmpl *template.Template, tags []string) (string, error) {
	u, err := url.Parse(item.Link)
	if err != nil {
		return "", err
	}
	buf := new(bytes.Buffer)
	err = tmpl.Execute(buf, item)
	if err != nil {
		return "", err
	}
	tootStr := buf.String()
	if len(tootStr) > kBlueskyMaxTootLen {
		tootStr = tootStr[:kBlueskyMaxTootLen]
	}
	post, err := skybot.NewPostBuilder(tootStr).
		WithExternalLink(item.Title, *u, item.Title).
		Build()
	if err != nil {
		return "", err
	}

	ctx := context.Background()
	cid, _, err := bpr.skyAgent.PostToFeed(ctx, post)
	if err != nil {
		return "", err
	}
	return cid, nil
}
