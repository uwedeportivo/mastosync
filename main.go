package main

import (
	"github.com/jomei/notionapi"
	"github.com/mattn/go-mastodon"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mmcdole/gofeed"
	"github.com/urfave/cli"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

const defaultTmplContent string = `{{.Title}}

{{.Description}}

{{.Link}}
`

func expandTilde(path string) (string, error) {
	if !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	return filepath.Join(usr.HomeDir, path[2:]), nil
}

func configDir(c *cli.Context) (string, error) {
	dir := c.String("config")
	if dir == "" {
		dir = ".mastosync"
		if _, err := os.Stat(dir); err != nil {
			usr, err := user.Current()
			if err != nil {
				return "", err
			}
			dir = filepath.Join(usr.HomeDir, ".mastosync")
		}
	}
	return expandTilde(dir)
}

func main() {
	app := cli.NewApp()
	app.Name = "mastosync"
	app.Usage = "Toot items from RSS feeds"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "config",
			Value: "",
			Usage: "path to a directory",
		},
	}

	app.Commands = []cli.Command{
		{
			Name:    "init",
			Aliases: []string{"i"},
			Usage:   "set up the sync directory",
			Action: func(c *cli.Context) error {
				dir, err := configDir(c)
				if err != nil {
					return err
				}
				err = os.Mkdir(dir, 0700)
				if err != nil {
					return err
				}
				err = os.Mkdir(filepath.Join(dir, "templates"), 0700)
				if err != nil {
					return err
				}
				err = os.WriteFile(filepath.Join(dir, "templates", "someA.tmpl"), []byte(defaultTmplContent),
					0600)
				if err != nil {
					return err
				}
				err = InitConfig(filepath.Join(dir, "config.yaml"))
				if err != nil {
					return err
				}
				err = CreateDB(filepath.Join(dir, "sync.sqlite3"))
				if err != nil {
					return err
				}
				return nil
			},
		},
		{
			Name:    "sync",
			Aliases: []string{"s"},
			Usage:   "sync RSS feed with mastodon",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "dryrun",
					Usage: "dryrun the sync",
				},
			},
			Action: func(c *cli.Context) error {
				dir, err := configDir(c)
				if err != nil {
					return err
				}
				cfg, err := ReadConfig(filepath.Join(dir, "config.yaml"))
				if err != nil {
					return err
				}
				dao, err := OpenDB(filepath.Join(dir, "sync.sqlite3"))
				if err != nil {
					return err
				}

				mClient := mastodon.NewClient(&cfg.Mas)
				syncer := Syncer{
					feedParser: gofeed.NewParser(),
					mClient:    mClient,
					dao:        dao,
					feeds:      cfg.Feeds,
					tmplDir:    filepath.Join(dir, "templates"),
					dryrun:     c.Bool("dryrun"),
				}
				return syncer.Sync()
			},
		},
		{
			Name:    "catchup",
			Aliases: []string{"c"},
			Usage:   "catchup DB with RSS feed",
			Action: func(c *cli.Context) error {
				dir, err := configDir(c)
				if err != nil {
					return err
				}
				cfg, err := ReadConfig(filepath.Join(dir, "config.yaml"))
				if err != nil {
					return err
				}
				dao, err := OpenDB(filepath.Join(dir, "sync.sqlite3"))
				if err != nil {
					return err
				}

				mClient := mastodon.NewClient(&cfg.Mas)
				syncer := Syncer{
					feedParser: gofeed.NewParser(),
					mClient:    mClient,
					dao:        dao,
					feeds:      cfg.Feeds,
					tmplDir:    filepath.Join(dir, "templates"),
				}
				return syncer.Catchup()
			},
		},
		{
			Name:    "save",
			Aliases: []string{"n"},
			Usage:   "save a status to notion",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "dryrun",
					Usage: "dryrun the sync",
				},
				cli.StringFlag{
					Name:  "id",
					Usage: "id of toot to save",
				},
				cli.StringFlag{
					Name:  "title",
					Usage: "title of the saved page",
				},
			},
			Action: func(c *cli.Context) error {
				dir, err := configDir(c)
				if err != nil {
					return err
				}
				cfg, err := ReadConfig(filepath.Join(dir, "config.yaml"))
				if err != nil {
					return err
				}

				notionClient := notionapi.NewClient(notionapi.Token(cfg.NotionToken), notionapi.WithRetry(2))
				mClient := mastodon.NewClient(&cfg.Mas)
				saver := Saver{
					mClient:        mClient,
					dryrun:         c.Bool("dryrun"),
					notionClient:   notionClient,
					notionParentID: cfg.NotionParent,
					pageTitle:      c.String("title"),
				}
				return saver.Save(mastodon.ID(c.String("id")))
			},
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
