package main

import (
	"bytes"
	"context"
	"html"
	"net/url"
	"text/template"

	skybot "github.com/danrusei/gobot-bsky"
	lexutil "github.com/bluesky-social/indigo/lex/util"
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
	buf := new(bytes.Buffer)
	err = tmpl.Execute(buf, item)
	if err != nil {
		return "", err
	}
	tootStr := buf.String()
	if len(tootStr) > kBlueskyMaxTootLen {
		tootStr = tootStr[:kBlueskyMaxTootLen]
	}
	post, err := skybot.NewPostBuilder(html.UnescapeString(tootStr)).
		WithExternalLink(html.UnescapeString(item.Title), *u, html.UnescapeString(item.Title), lexutil.LexBlob{}).
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
