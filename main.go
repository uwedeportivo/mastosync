package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/jomei/notionapi"
	"github.com/mattn/go-mastodon"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mmcdole/gofeed"
	"github.com/pkg/browser"
	"github.com/urfave/cli"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"log"
	"net/http"
	"net/url"
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

func handleOauthCallback(codeChan chan string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		queryParts, _ := url.ParseQuery(r.URL.RawQuery)

		// Use the authorization code that is pushed to the redirect URL.
		code := queryParts["code"][0]

		// write the authorization code to the channel
		codeChan <- code

		msg := "<p><strong>Authentication successful</strong>. You may now close this tab.</p>"
		// send a success message to the browser
		if _, err := fmt.Fprint(w, msg); err != nil {
			log.Println(err)
		}
	}
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
					Usage: "dryrun the save",
				},
				cli.BoolFlag{
					Name:  "debug",
					Usage: "debug the save",
				},
				cli.BoolFlag{
					Name:  "external",
					Usage: "do not use gdrive to store media, rely on external mastodon server",
				},
				cli.StringFlag{
					Name:  "title",
					Usage: "title of the saved page",
				},
			},
			Action: func(c *cli.Context) error {
				if !c.Args().Present() {
					return fmt.Errorf("missing toot id or url to save")
				}
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

				b, err := os.ReadFile(filepath.Join(dir, "gdrive.json"))
				if err != nil {
					return err
				}

				// If modifying these scopes, delete your previously saved token.json.
				gdriveConfig, err := google.ConfigFromJSON(b, drive.DriveFileScope)
				if err != nil {
					return err
				}

				tokenFile, err := os.Open(filepath.Join(dir, "gdrive.token"))
				if err != nil {
					return err
				}
				gdriveToken := &oauth2.Token{}
				err = json.NewDecoder(tokenFile).Decode(gdriveToken)
				if err != nil {
					return err
				}
				err = tokenFile.Close()
				if err != nil {
					return err
				}
				gdriveClient := gdriveConfig.Client(context.Background(), gdriveToken)

				gdriveService, err := drive.NewService(context.Background(),
					option.WithHTTPClient(gdriveClient))
				if err != nil {
					return err
				}

				saver := Saver{
					mClient:        mClient,
					dryrun:         c.Bool("dryrun"),
					notionClient:   notionClient,
					notionParentID: cfg.NotionParent,
					pageTitle:      c.String("title"),
					gdriveService:  gdriveService,
					debug:          c.Bool("debug"),
					usegdrive:      !c.Bool("external"),
					bridge:         cfg.Bridge,
					parent:         cfg.Parent,
				}
				return saver.SaveToot(c.Args().First())
			},
		},
		{
			Name:    "auth",
			Aliases: []string{"a"},
			Usage:   "refresh oauth token for Google Drive",
			Action: func(c *cli.Context) error {
				dir, err := configDir(c)
				if err != nil {
					return err
				}
				b, err := os.ReadFile(filepath.Join(dir, "gdrive.json"))
				if err != nil {
					return err
				}

				// If modifying these scopes, delete your previously saved token.json.
				gdriveConfig, err := google.ConfigFromJSON(b, drive.DriveFileScope)
				if err != nil {
					return err
				}

				tr := &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				}
				sslcli := &http.Client{Transport: tr}
				ctx := context.Background()
				ctx = context.WithValue(ctx, oauth2.HTTPClient, sslcli)

				server := &http.Server{Addr: ":9999"}

				// create a channel to receive the authorization code
				codeChan := make(chan string)

				http.HandleFunc("/oauth/callback", handleOauthCallback(codeChan))

				go func() {
					if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
						log.Fatalf("Failed to start server: %v", err)
					}
				}()

				// get the OAuth authorization URL
				authUrl := gdriveConfig.AuthCodeURL("state-token", oauth2.AccessTypeOffline)

				// Redirect user to consent page to ask for permission
				// for the scopes specified above
				fmt.Printf("Your browser has been opened to visit::\n%s\n", authUrl)

				// open user's browser to login page
				if err := browser.OpenURL(authUrl); err != nil {
					panic(fmt.Errorf("failed to open browser for authentication %v", err))
				}

				authCode := <-codeChan
				tok, err := gdriveConfig.Exchange(context.Background(), authCode)
				if err != nil {
					return err
				}
				f, err := os.OpenFile(filepath.Join(dir, "gdrive.token"),
					os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
				if err != nil {
					return err
				}
				err = json.NewEncoder(f).Encode(tok)
				if err != nil {
					return err
				}
				return f.Close()
			},
		},
		{
			Name:    "mandala",
			Aliases: []string{"m"},
			Usage:   "post a mandala",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "skip_post",
					Usage: "generate mandala but don't post",
				},
				cli.BoolFlag{
					Name:  "skip_generate",
					Usage: "post an exisiting mandala",
				},
				cli.StringFlag{
					Name:  "path",
					Value: "/tmp/mandala.png",
					Usage: "path to mandala",
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

				mClient := mastodon.NewClient(&cfg.Mas)
				mandala := Mandala{
					mClient:      mClient,
					scriptPath:   cfg.Mandala,
					mandalaPath:  c.String("path"),
					skipPost:     c.Bool("skip_post"),
					skipGenerate: c.Bool("skip_generate"),
				}
				return mandala.Post()
			},
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
