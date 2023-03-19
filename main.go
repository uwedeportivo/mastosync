package main

import (
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
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
