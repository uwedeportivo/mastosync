package main

import (
	"bytes"
	"context"
	"html"
	"log"
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

// featureImageURL returns the URL of the item's <media:content> image, as
// emitted by sagenhaft's rss.xml for posts with a page-bundle "feature*"
// image, or "" if the item has none.
func featureImageURL(item *gofeed.Item) string {
	for _, e := range item.Extensions["media"]["content"] {
		if u := e.Attrs["url"]; u != "" {
			return u
		}
	}
	return ""
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

	ctx := context.Background()

	external := &appbsky.EmbedExternal_External{
		Title:       html.UnescapeString(item.Title),
		Uri:         u.String(),
		Description: html.UnescapeString(item.Title),
	}

	if imgURL := featureImageURL(item); imgURL != "" {
		if parsed, err := url.Parse(imgURL); err != nil {
			log.Printf("failed to parse feature image url %q: %v", imgURL, err)
		} else if thumb, err := bpr.skyAgent.UploadImage(ctx, skybot.Image{Title: external.Title, Uri: *parsed}); err != nil {
			log.Printf("failed to upload feature image %q to bluesky: %v", imgURL, err)
		} else {
			external.Thumb = thumb
		}
	}

	post := appbsky.FeedPost{
		LexiconTypeID: "app.bsky.feed.post",
		Text:          html.UnescapeString(tootStr),
		CreatedAt:     time.Now().Format(time.RFC3339),
		Embed: &appbsky.FeedPost_Embed{
			EmbedExternal: &appbsky.EmbedExternal{
				LexiconTypeID: "app.bsky.embed.external",
				External:      external,
			},
		},
	}

	cid, _, err := bpr.skyAgent.PostToFeed(ctx, post)
	if err != nil {
		return "", err
	}
	return cid, nil
}
