# Walker

Walker walkes aka as crawls through websites and collects performance and SEO relevant data. The results can be browsed through a very simple web interface. Apart from that they are exposed as prometheus metrics (not implemented yet).

**Be careful when crawling your website with walker with aggressive settings, it might take your site down**

## Configuration

```yaml
---
# target of your scrape
target: http://www.bestbytes.de
# number of concurrent go routines
concurrency: 2
# where to run the webinterface
addr: ":3001"
# if you want to ignore <meta name="robots" content="noindex,nofollow"/>
ignorerobots: true
# in some cases using cookies is friendlier to the server
usecookies: true

# ignoring urls
## based on query parameters in this example all links, that contain a queryparameter foo
ignorequerieswith:
  - foo
## skip everything that has a query
ignoreallqueries: true
# what paths (that would be a prefixes)
ignore:
  - /foomo
...
```

## error detection

- everything greater than 400 will be tracked as an error

## external link validation (not implemented yet)

- check external links
- forbidden sites like a stage system 

## seo validation

- missing title, description, h1
- duplication title, description, h1

### seo validation schemata

WIP

## metrics

Work in progress exposed on /metrics

- vector of status codes
- performance buckets