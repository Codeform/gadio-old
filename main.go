package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	re "regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eduncan911/podcast"
	"github.com/gocolly/colly"
	"github.com/gocolly/colly/debug"
)

const (
	base     = "https://www.gcores.com/"
	domain   = "www.gcores.com"
	fmtPubIn = "2006-01-02"
)

func parseArgs() (int, int) {
	start := flag.Int("from", 1, "starting page number")
	end := flag.Int("to", 2, "ending page number")
	purge := flag.Bool("purge", false, "purge cached scraps")
	flag.Parse()
	if *purge {
		os.RemoveAll(".cache")
	}
	if !(*start >= 1 && *start <= *end) {
		panic("Wrong starting page")
	}
	return *start, *end
}

func main() {
	f, t := parseArgs()
	logs, _ := os.Create("results/log.txt")
	defer logs.Close()

	now := time.Now()
	then := time.Date(2010, time.May, 10, 12, 00, 00, 00, now.Location())

	feed := podcast.New("（非官方源）GADIO",
		base,
		"（非官方源）机核网 gcores.com 「不止是游戏」，机核从年轻人的兴趣出发，旨在用亲切幽默地风格在游戏、电影、音乐、消费等不同领域提供有价值的精致内容，把有着各种兴趣爱好的年轻人聚集在一起，在这个无趣的世界里，找到一处属于自己的安宁之乡。",
		&then,
		&now)
	feed.Copyright = domain
	feed.Generator = "custom golang application based on github.com/eduncan911/podcast"
	feed.Language = "zh-cn"
	feed.SkipDays = "30"
	feed.ManagingEditor = "gamecores@qq.com (机核网 www.gcores.com)"
	feed.AddCategory("Games & Hobbies", []string{"Video Games"})
	feed.AddAuthor("机核网 www.gcores.com", "gamecores@qq.com")
	feed.AddSubTitle("机核网－嘉电游GADIO")
	feed.AddImage("http://media.fmit.cn/feed/gadionewlogos.png")
	feed.AddCategory("Games & Hobbies", []string{"Video Games"})

	ptnPage := re.MustCompile("/radios\\?page=\\d*$")
	ptnEnclosure := re.MustCompile("/radios/\\d*$")
	ptnNum := re.MustCompile("\\d*$")
	ptnCovUrl := re.MustCompile("http.*\\.jpg")

	pageCrawler := colly.NewCollector(
		colly.AllowedDomains(domain),
		colly.UserAgent("unknown"),
		colly.CacheDir(".cache"),
		colly.Debugger(&debug.LogDebugger{
			Output: logs,
		}),
	)

	pageCrawler.Limit(&colly.LimitRule{
		Delay:       time.Microsecond * 800,
		RandomDelay: time.Microsecond * 600,
		Parallelism: 4,
	})

	enclosureCrawler := pageCrawler.Clone()

	var lck sync.Mutex

	pageCrawler.OnRequest(func(req *colly.Request) {
		log.Println("Visiting page", req.URL)
	})

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
						pageCtx.Put("publish", now)
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
					pageCtx.Put("category", coe.ChildText("a"))
				}
			}
		})

		e.ForEach("a[href]", func(_ int, coe *colly.HTMLElement) {
			if ptnEnclosure.MatchString(coe.Attr("href")) {
				enclosureCrawler.Request("GET",
					e.Request.AbsoluteURL(coe.Attr("href")),
					nil, pageCtx, nil)
			}
		})
	})

	pageCrawler.OnHTML("a[href]", func(e *colly.HTMLElement) {
		link := e.Attr("href")

		if ptnPage.MatchString(link) {
			if num, _ := strconv.Atoi(ptnNum.FindString(link)); num <= t {
				e.Request.Visit(link)
			}
		}
	})

	enclosureCrawler.OnHTML("a.originalButton.originalButton-circle.ml-3", func(e *colly.HTMLElement) {
		e.Request.Ctx.Put("audio", e.Attr("href"))
	})

	enclosureCrawler.OnHTML("span[data-text]", func(e *colly.HTMLElement) {
		e.Request.Ctx.Put("summary", e.Request.Ctx.Get("summary")+"\r\n"+e.Text)
	})

	enclosureCrawler.OnHTML("h1.originalPage_title", func(e *colly.HTMLElement) {
		e.Request.Ctx.Put("title", e.Text)
	})

	enclosureCrawler.OnScraped(func(rsp *colly.Response) {
		lck.Lock()
		item := createFeedItem(rsp.Ctx)
		feed.AddItem(item)
		lck.Unlock()

		log.Println("Return with scrapped podcast", item.PubDate, ":", item.Title)
	})

	pageCrawler.Visit(base + "radios?page=" + strconv.Itoa(f))

	pageCrawler.Wait()
	enclosureCrawler.Wait()

	results, _ := os.Create("results/gadio.xml")
	feed.Encode(results)
	results.Close()
}

func createFeedItem(ctx *colly.Context) podcast.Item {
	var item podcast.Item
	rsp, err := http.Get(ctx.Get("audio"))
	if err != nil {
		return item
	}
	defer rsp.Body.Close()

	if _, exist := rsp.Header["Content-Length"]; !exist {
		return item
	}

	pub := ctx.GetAny("publish").(time.Time)
	length, _ := strconv.Atoi(rsp.Header["Content-Length"][0])

	item.Title = ctx.Get("title") + " | " + ctx.Get("category")

	item.Description = ctx.Get("summary")

	item.Link = domain

	item.AddImage(ctx.Get("cover"))

	item.AddPubDate(&pub)

	item.AddEnclosure(ctx.Get("audio"), func(str string) podcast.EnclosureType {
		switch str {
		case "audio/x-m4a":
			return podcast.M4A
		case "video/x-m4v":
			return podcast.M4V
		case "video/mp4":
			return podcast.MP4
		case "audio/mpeg":
			return podcast.MP3
		case "video/quicktime":
			return podcast.MOV
		case "application/pdf":
			return podcast.PDF
		case "document/x-epub":
			return podcast.EPUB
		default:
			return -1
		}
	}(rsp.Header["Content-Type"][0]), int64(length))

	item.AddDuration(parseDuration(ctx.Get("duration")))

	return item
}

func parseDuration(str string) int64 {
	if str == "" {
		return 3600 + 22*60 + 18
	}
	var dur int64
	seq := strings.Split(str, ":")
	t, _ := strconv.Atoi(seq[0])
	dur += int64(t) * 3600
	t, _ = strconv.Atoi(seq[1])
	dur += int64(t) * 60
	if len(seq) == 3 {
		t, _ = strconv.Atoi(seq[2])
	}
	dur += int64(t)
	return dur
}
