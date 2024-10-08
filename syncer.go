package main

import (
	"fmt"
	"path/filepath"
	"text/template"
	"time"

	"github.com/mmcdole/gofeed"
)

type Syncer struct {
	feedParser *gofeed.Parser
	poster     Poster
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
		if syncer.dryrun {
			fmt.Println("would be tooting:\n", item.Title)
			alreadyProcessed[item.GUID] = item
			continue
		}
		postID, err := syncer.poster.Post(item, tmpl)
		if err != nil {
			return err
		}

		err = syncer.dao.RecordSync(item.GUID, postID, time.Now())
		if err != nil {
			return err
		}
		alreadyProcessed[item.GUID] = item
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
