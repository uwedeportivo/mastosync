# mastosync

> Disclaimer: Please use this small script at your own risk. I wrote it up quickly in one afternoon and make no guarantees to its 
> usefulness, correctness etc.

# Introduction

_mastosync_ is a small command-line utility that reads RSS feeds and toots items from those feeds to a specified mastodon instance. 
Functionality is similar to [mastofeed](https://mastofeed.org/) but as a CLI instead of a comfortable web service. If you are looking
for a no-hassle way to automatically toot RSS items, then check out **mastofeed**.

# Setup

You need to set up a sync directory. You can do that easily by running

```sh
mastosync init
```

It will create the ".mastosync" directory. In there you will find the file "config.yaml". You need to specify your mastodon credentials
and your RSS feeds together with toot templates that go with each feed. The RSS feeds list order is respected and important.
If you have subfeeds in your blog with specific tags, you can specify them ahead of the general blog feed
and specify the tags you want in your toots in the templates. That way mastosync will use the
more specific templates instead of the general template without the tags.

# Catchup

Presumably you already have RSS feeds and don't want _mastosync_ to suddenly toot out all the items in them. To avoid that you
should let _mastosync_ catchup:

```sh
mastosync catchup
```

This will record all the existing items in the RSS feeds as already "tooted".

# Sync

After "init" and "catchup", you can run 

```sh
mastosync sync
```

and if there are new items in any of the specified feeds, they will be tooted. You can "--dryrun" the sync first.

Use it in a cronjob or similar for hands-free syncing between RSS and Mastodon.

