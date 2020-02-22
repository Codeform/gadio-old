package main

import (
	"fmt"
	"os"
	re "regexp"
	"strconv"
	"time"

	"github.com/gocolly/colly"
	"github.com/gocolly/colly/debug"
	"github.com/gocolly/colly/proxy"
	sl "github.com/sean-public/fast-skiplist"
)

const (
	base      = "https://www.gcores.com/"
	domain    = "www.gcores.com"
	fmtPubIn  = "2006-01-02"
	fmtPubOut = "Mon, 02 Jan 2006 15:04:05"
)

type closure struct {
	publish time.Time //
	cover,  //
	duration, //
	catalog, //
	title,
	summary,
	audio,
	bytes string
}

func main() {
	logs, _ := os.Create("./results/log.txt")
	defer logs.Close()

	ptnPage := re.MustCompile("/radios\\?page=\\d*$")
	ptnClosure := re.MustCompile("/radios/\\d*$")
	ptnNum := re.MustCompile("\\d*$")
	ptnCovUrl := re.MustCompile("http.*\\.jpg")

	pageCrawler := colly.NewCollector(
		colly.AllowedDomains(domain),
		colly.UserAgent("unknown"),
		colly.CacheDir("./.cache"),
		colly.Debugger(&debug.LogDebugger{
			Output: logs,
		}),
	)

	if prx, err := proxy.RoundRobinProxySwitcher(
		"http://171.35.212.106:9999",
		"http://123.169.102.37:9999",
		"http://110.243.8.36:9999",
		"http://118.212.106.252:9999",
		"http://163.204.247.170:9999",
	); err == nil {
		pageCrawler.SetProxyFunc(prx)
	}

	pageCrawler.Limit(&colly.LimitRule{
		Delay:       time.Microsecond * 800,
		RandomDelay: time.Microsecond * 600,
		Parallelism: 4,
	})

	closureCrawler := pageCrawler.Clone()

	closures := sl.New()

	pageCrawler.OnHTML("div.col-xl-3.col-md-4.col-sm-6", func(e *colly.HTMLElement) {
		pageCtx := colly.NewContext()

		//duration:class="original_imgArea_info"
		e.ForEach("*[class]", func(_ int, coe *colly.HTMLElement) {
			switch coe.Attr("class") {
			case "original_createdDate":
				{
					if pub, err := time.Parse(fmtPubIn, coe.Text); err == nil {
						pageCtx.Put("publish", pub)
					} else {
						pageCtx.Put("publish", time.Now())
					}
				}
			case "original_imgArea":
				{
					if coe.Attr("style") != "" {
						pageCtx.Put("cover", ptnCovUrl.FindString(coe.Attr("style")))
					}
				}
			case "original_imgArea_info":
				{
					pageCtx.Put("duration", coe.Text)
				}
			case "original_category":
				{
					pageCtx.Put("catalog", coe.ChildText("a"))
				}
			}
		})

		e.ForEach("a[href]", func(_ int, coe *colly.HTMLElement) {
			if ptnClosure.MatchString(coe.Attr("href")) {
				closureCrawler.Request("GET",
					e.Request.AbsoluteURL(coe.Attr("href")),
					nil, pageCtx, nil)
			}
		})
	})

	pageCrawler.OnHTML("a[href]", func(e *colly.HTMLElement) {
		link := e.Attr("href")

		if ptnPage.MatchString(link) {
			if num, _ := strconv.Atoi(ptnNum.FindString(link)); num <= 1 {
				e.Request.Visit(link)
			}
		}
	})

	closureCrawler.OnRequest(func(req *colly.Request) {
		fmt.Println(req.Ctx.Get("catalog"))
	})

	pageCrawler.Visit(base + "radios")

	results, _ := os.Create("./results/raw.xml")
	defer results.Close()
	for cursor := closures.Front(); cursor != nil; cursor = cursor.Next() {
		results.WriteString(cursor.Value().(string))
	}
	results.Sync()
}
