package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/url"
	"path/filepath"
	"text/template"
	"time"

	skybot "github.com/danrusei/gobot-bsky"
	mdon "github.com/mattn/go-mastodon"
	"github.com/mmcdole/gofeed"
)

const kMaxBlueSkyGraphemes = 300

type Syncer struct {
	feedParser *gofeed.Parser
	mClient    *mdon.Client
	skyAgent   *skybot.BskyAgent
	dao        *DAO
	feeds      []FeedTemplatePair
	tmplDir    string
	dryrun     bool
}

func (syncer *Syncer) Sync() error {
	alreadyProcessed := make(map[string]*gofeed.Item)
	for _, feedTmplPair := range syncer.feeds {
		err := syncer.SyncFeed(feedTmplPair.FeedURL, feedTmplPair.Template, alreadyProcessed)
		if err != nil {
			return err
		}
	}
	return nil
}

func (syncer *Syncer) SyncFeed(feedURL string, templatePath string,
	alreadyProcessed map[string]*gofeed.Item) error {
	feed, err := syncer.feedParser.ParseURL(feedURL)
	if err != nil {
		return err
	}

	tmpl, err := template.ParseFiles(filepath.Join(syncer.tmplDir, templatePath))
	if err != nil {
		return err
	}

	var outstandingItems []*gofeed.Item
	for _, item := range feed.Items {
		_, ap := alreadyProcessed[item.GUID]
		if ap {
			continue
		}
		toot, err := syncer.dao.FindToot(item.GUID)
		if err != nil {
			return err
		}

		if toot == nil {
			outstandingItems = append(outstandingItems, item)
		} else {
			break
		}
	}
	for _, item := range outstandingItems {
		buf := new(bytes.Buffer)
		err = tmpl.Execute(buf, item)
		if err != nil {
			return err
		}
		tootStr := buf.String()
		if len(tootStr) > 500 {
			tootStr = tootStr[:500]
		}
		if syncer.dryrun {
			fmt.Println("would be tooting:\n", tootStr)
			alreadyProcessed[item.GUID] = item
			continue
		} else {
			fmt.Println("tooting:\n", tootStr)
		}
		toot := mdon.Toot{
			Status: tootStr,
		}

		status, err := syncer.mClient.PostStatus(context.Background(), &toot)
		if err != nil {
			return err
		}

		err = syncer.dao.RecordSync(item.GUID, string(status.ID), time.Now())
		if err != nil {
			return err
		}
		alreadyProcessed[item.GUID] = item

		if syncer.skyAgent != nil {
			err = syncer.PostToBlueSky(item)
			if err != nil {
				log.Println("posting to bluesky failed:", err, item.GUID)
			}
		}
	}
	return nil
}

func (syncer *Syncer) Catchup() error {
	for _, feedTmplPair := range syncer.feeds {
		err := syncer.CatchupFeed(feedTmplPair.FeedURL)
		if err != nil {
			return err
		}
	}
	return nil
}

func (syncer *Syncer) CatchupFeed(feedURL string) error {
	feed, err := syncer.feedParser.ParseURL(feedURL)
	if err != nil {
		return err
	}

	now := time.Now()
	for _, item := range feed.Items {
		err = syncer.dao.RecordSync(item.GUID, "catchup", now)
		if err != nil {
			return err
		}
	}
	return nil
}

func (syncer *Syncer) PostToBlueSky(item *gofeed.Item) error {
	u, err := url.Parse(item.Link)
	if err != nil {
		return err
	}
	cappedLength := len(item.Description)
	if cappedLength > kMaxBlueSkyGraphemes {
		cappedLength = kMaxBlueSkyGraphemes
	}
	post, err := skybot.NewPostBuilder(item.Description[:cappedLength]).
		WithExternalLink(item.Title, *u, item.Title).
		Build()
	if err != nil {
		return err
	}

	ctx := context.Background()
	_, _, err = syncer.skyAgent.PostToFeed(ctx, post)
	if err != nil {
		return err
	}
	return nil
}
