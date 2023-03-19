package main

import (
	"bytes"
	"context"
	"github.com/mattn/go-mastodon"
	"github.com/mmcdole/gofeed"
	"path/filepath"
	"text/template"
	"time"
)

type Syncer struct {
	feedParser *gofeed.Parser
	mClient    *mastodon.Client
	dao        *DAO
	feeds      []FeedTemplatePair
	tmplDir    string
}

func (syncer *Syncer) Sync() error {
	for _, feedTmplPair := range syncer.feeds {
		err := syncer.SyncFeed(feedTmplPair.FeedURL, feedTmplPair.Template)
		if err != nil {
			return err
		}
	}
	return nil
}

func (syncer *Syncer) SyncFeed(feedURL string, templatePath string) error {
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
		toot := mastodon.Toot{
			Status: buf.String(),
		}

		status, err := syncer.mClient.PostStatus(context.Background(), &toot)
		if err != nil {
			return err
		}

		err = syncer.dao.RecordSync(item.GUID, string(status.ID), time.Now())
		if err != nil {
			return err
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
