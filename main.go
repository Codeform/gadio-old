package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	re "regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/beevik/etree"
	"github.com/gocolly/colly"
	"github.com/gocolly/colly/debug"
	sl "github.com/sean-public/fast-skiplist"
)

const (
	base      = "https://www.gcores.com/"
	domain    = "www.gcores.com"
	fmtPubIn  = "2006-01-02"
	fmtPubOut = "Mon, 02 Jan 2006 15:04:05 +0000"
)

type closure struct {
	publish, //
	cover, //
	duration, //
	catalog, //
	title, //
	summary, //
	audio, //
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

	// if prx, err := proxy.RoundRobinProxySwitcher(
	// 	"http://171.35.212.106:9999",
	// 	"http://123.169.102.37:9999",
	// 	"http://110.243.8.36:9999",
	// 	"http://118.212.106.252:9999",
	// 	"http://163.204.247.170:9999",
	// ); err == nil {
	// 	pageCrawler.SetProxyFunc(prx)
	// }

	pageCrawler.Limit(&colly.LimitRule{
		Delay:       time.Microsecond * 800,
		RandomDelay: time.Microsecond * 600,
		Parallelism: 4,
	})

	closureCrawler := pageCrawler.Clone()
	audioHeader := pageCrawler.Clone()

	fmt.Println(pageCrawler.ID, "", closureCrawler.ID, "", audioHeader.ID)

	closures := sl.New()
	var lck sync.Mutex

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
			if num, _ := strconv.Atoi(ptnNum.FindString(link)); num <= 10 && num >= 1 {
				e.Request.Visit(link)
			}
		}
	})

	closureCrawler.OnHTML("a.originalButton.originalButton-circle.ml-3", func(e *colly.HTMLElement) {
		e.Request.Ctx.Put("audio", e.Attr("href"))
	})

	closureCrawler.OnHTML("span[data-text]", func(e *colly.HTMLElement) {
		e.Request.Ctx.Put("summary", e.Text)
	})

	closureCrawler.OnHTML("h1.originalPage_title", func(e *colly.HTMLElement) {
		e.Request.Ctx.Put("title", e.Text)
	})

	closureCrawler.OnScraped(func(rsp *colly.Response) {
		key := float64(rsp.Ctx.GetAny("publish").(time.Time).Unix()) * -1
		lck.Lock()
		closures.Set(key,
			&closure{
				publish:  rsp.Ctx.GetAny("publish").(time.Time).Format(fmtPubOut),
				cover:    rsp.Ctx.Get("cover"),
				duration: rsp.Ctx.Get("duration"),
				catalog:  rsp.Ctx.Get("catalog"),
				title:    escape(rsp.Ctx.Get("title")),
				summary:  escape(rsp.Ctx.Get("summary")),
				audio:    rsp.Ctx.Get("audio"),
			})
		lck.Unlock()
	})

	pageCrawler.Visit(base + "radios")

	pageCrawler.Wait()
	closureCrawler.Wait()

	rsltXml := etree.NewDocument()

	for cursor := closures.Front(); cursor != nil; cursor = cursor.Next() {
		if c := cursor.Value().(*closure).Marshal(); c != nil {
			rsltXml.AddChild(c)
		}
	}
	results, _ := os.Create("./results/gadio.xml")
	rsltXml.WriteTo(results)
	results.WriteString(`
	</channel>
	</rss>
	`)
	results.Seek(0, io.SeekStart)
	results.WriteString(`
	<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"
	xmlns:content="http://purl.org/rss/1.0/modules/content/"
	xmlns:wfw="http://wellformedweb.org/CommentAPI/"
	xmlns:dc="http://purl.org/dc/elements/1.1/"
	xmlns:atom="http://www.w3.org/2005/Atom"
	xmlns:sy="http://purl.org/rss/1.0/modules/syndication/"
	xmlns:slash="http://purl.org/rss/1.0/modules/slash/"
	xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd"
xmlns:rawvoice="http://www.rawvoice.com/rawvoiceRssModule/"
>

<channel>
	<title>机核网 GADIO 游戏广播（非官方源）</title>
	<atom:link href="https://pagini-engine.com/feed/gadio.xml" rel="self" type="application/rss+xml" />
	<link>https://pagini-engine.com/feed/gadio.xml</link>
	<description>机核网 www.gcores.com</description>
	<lastBuildDate>` + time.Now().Format(fmtPubOut) + `</lastBuildDate>

	<language>zh-cn</language>
	<sy:updatePeriod>monthly</sy:updatePeriod>
	<sy:updateFrequency>30</sy:updateFrequency>
	<generator>dj-podcaster</generator>
	<itunes:summary>机核网 gcores.com 「不止是游戏」</itunes:summary>
	<itunes:author>机核网 www.gcores.com</itunes:author>

	<itunes:explicit>clean</itunes:explicit>
	<itunes:image href="http://media.fmit.cn/feed/gadionewlogos.png" />
	<itunes:owner>
		<itunes:name>机核网 www.gcores.com</itunes:name>
		<itunes:email>gamecores@qq.com</itunes:email>
	</itunes:owner>
	<managingEditor>gamecores@qq.com (机核网 www.gcores.com)</managingEditor>

	<copyright>www.gcores.com</copyright>
	<itunes:subtitle>机核网－嘉电游GADIO</itunes:subtitle>
	<itunes:keywords>游戏,Xbox360,PS3,Wii,3DS,PSP2,宅男,机核网,游戏电台,心得,攻略,采访,游戏论坛</itunes:keywords>
	<!--itunes:category text="Games &amp; Hobbies">
		<itunes:category text="Video Games" />
	</itunes:category-->
	<itunes:category text="Games &amp; Hobbies">
	<itunes:category text="Video Games" />
</itunes:category>
	`)
	results.Close()
}

func escape(str string) string {
	str = strings.ReplaceAll(str, "&", "&amp")
	str = strings.ReplaceAll(str, ">", "&gt")
	str = strings.ReplaceAll(str, "<", "&lt")
	str = strings.ReplaceAll(str, "'", "&apos")
	str = strings.ReplaceAll(str, "\"", "&quot")
	return str
}

func (cl *closure) Marshal() *etree.Element {
	elem := etree.NewElement("item")
	var iter *etree.Element

	iter = elem.CreateElement("title")
	iter.SetText(cl.title + " " + cl.catalog)

	iter = elem.CreateElement("pubDate")
	iter.SetText(cl.publish)

	iter = elem.CreateElement("itunes:duration")
	iter.SetText(cl.duration)

	iter = elem.CreateElement("guid")
	iter.CreateAttr("isPermaLink", "false")
	iter.SetText(cl.audio)

	iter = elem.CreateElement("description")
	iter.SetText("<![CDATA[" + cl.summary + "]]>")

	iter = elem.CreateElement("wfw:commentRss")
	iter.SetText("https://pagini-engine.com/feed/gadio.xml")

	iter = elem.CreateElement("slash:comments")
	iter.SetText("0")

	iter = elem.CreateElement("itunes:image")
	iter.SetText(cl.audio)

	iter = elem.CreateElement("enclosure")
	rsp, err := http.Head(cl.audio)
	if err != nil {
		return nil
	}
	iter.CreateAttr("url", cl.audio)
	iter.CreateAttr("length", rsp.Header["Content-Length"][0])
	iter.CreateAttr("type", rsp.Header["Content-Type"][0])

	iter = elem.CreateElement("itunes:subtitle")
	iter.SetText(cl.summary)

	iter = elem.CreateElement("itunes:summary")
	iter.SetText(cl.summary)

	iter = elem.CreateElement("itunes:author")
	iter.SetText("www.gcores.com")

	iter = elem.CreateElement("itunes:explicit>")
	iter.SetText("clean")

	return elem
}
