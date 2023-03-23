package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"
)

func main() {
	// 禁用chrome headless
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("window-position", "2500,0"),
	)
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	// create context
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	r, w := io.Pipe()

	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	// 设置输出文件名为当前目录下的 video.mp4
	outputFile := fmt.Sprintf("%s/video.mp4", cwd)

	// Build the ffmpeg command.
	cmd := exec.Command("ffmpeg",
		"-y",
		"-f", "image2pipe",
		"-r", fmt.Sprintf("%d", 25),
		"-i", "-",
		"-c:v", "libx264",
		//"-preset", "slow",
		//"-crf", "18",
		//"-pix_fmt", "yuv420p",
		"-f", "mp4",
		"-movflags", "faststart",
		outputFile,
	)
	cmd.Stdin = r

	err = cmd.Start()
	if err != nil {
		log.Fatalf("start err %v", err)
	}

	var wg sync.WaitGroup

	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch ev.(type) {
		case *page.EventScreencastFrame:
			wg.Add(1)
			go func() {
				defer wg.Done()
				c := chromedp.FromContext(ctx)
				event, ok := ev.(*page.EventScreencastFrame)
				if !ok {
					log.Fatalf("assert err %v", event)
				}
				err := page.ScreencastFrameAck(event.SessionID).Do(cdp.WithExecutor(ctx, c.Target))
				if err != nil {
					log.Fatalf("ack err %v", err)
					return
				}
				d, err := base64.StdEncoding.DecodeString(event.Data)
				if err != nil {
					log.Fatalf("decode err %v", err)
					return
				}
				metaData, _ := json.Marshal(event.Metadata)
				log.Printf("frame -> %v %s", event.SessionID, metaData)
				_, err = w.Write(d)
				if err != nil {
					log.Printf("write err %v", err)
					return
				}
			}()
		}
	})

	err = chromedp.Run(ctx, chromedp.Tasks{
		chromedp.ActionFunc(func(ctx context.Context) error {
			cookies, err := loadCookie()
			if err != nil {
				return err
			}
			err = network.SetCookies(cookies).Do(ctx)
			if err != nil {
				return err
			}

			x, _ := chromedp.Targets(ctx)

			y, _ := json.Marshal(x)

			log.Printf("obt %s", y)

			return nil
		}),
	})

	if err != nil {
		log.Fatalf("run _blank err %v", err)
	}

	log.Printf("run 1")
	err = chromedp.Run(ctx, chromedp.Tasks{
		chromedp.Navigate("https://www.xiaohongshu.com/explore/640734780000000013032117"),
		// 等待播放按钮出现
		chromedp.WaitVisible(`//*[@id="videoPlayer"]/xg-start/div[@class="xgplayer-icon-play"]`),
		// 点击播放按钮
		chromedp.Click(`//*[@id="videoPlayer"]/xg-start/div[@class="xgplayer-icon-play"]`),
		// 等待loading消失
		chromedp.WaitNotVisible(`//*[@id="videoPlayer"]/xg-loading`),
		// 保证进度条是始终出现
		//chromedp.Evaluate(`
		//function x(xpath) {
		//	var result = document.evaluate(xpath, document, null, XPathResult.ANY_TYPE, null);
		//	return result.iterateNext()
		//}`, nil),
		//chromedp.Evaluate(`x('//*[@id="videoPlayer"]').classList.add("xgplayer-inactive")`, nil),
		// 开始录制
		page.StartScreencast().WithFormat("jpeg"),
		// 5秒
		chromedp.Sleep(5 * time.Second),
		// 结束录制
		page.StopScreencast(),
		// 停止视频文件管道写入
		chromedp.ActionFunc(func(ctx context.Context) error {

			wg.Wait()

			err = w.Close()
			if err != nil {
				log.Fatalf("w close err %v", err)
			}
			log.Printf("close succ")
			err := cmd.Wait()
			if err != nil {
				log.Printf("wait err %v", err)
				return err
			}
			log.Printf("cmd end")
			return nil
		}),
	})

	if err != nil {
		log.Fatalf("run err %v", err)
	}

	//time.Sleep(10 * time.Second)

	log.Printf("succ")
}

type cookieItem struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expirationDate"`
	Secure   bool    `json:"secure"`
	HttpOnly bool    `json:"httpOnly"`
	Session  bool    `json:"session"`
	SameSite string  `json:"sameSite"`
	StoreID  string  `json:"storeId"`
	HostOnly bool    `json:"hostOnly"`
}

// loadCookie ...
func loadCookie() ([]*network.CookieParam, error) {
	body, err := ioutil.ReadFile("auth.json")
	if err != nil {
		return nil, err
	}

	cookies := []*cookieItem{}

	err = json.Unmarshal(body, &cookies)
	if err != nil {
		return nil, err
	}

	ret := []*network.CookieParam{}
	for _, cookie := range cookies {
		t := cdp.TimeSinceEpoch(time.Unix(int64(cookie.Expires), 0))

		ret = append(ret, &network.CookieParam{
			Name:     cookie.Name,
			Value:    cookie.Value,
			URL:      "",
			Domain:   cookie.Domain,
			Path:     cookie.Path,
			Secure:   cookie.Secure,
			HTTPOnly: cookie.HttpOnly,
			SameSite: network.CookieSameSite(cookie.SameSite),
			Expires:  &t,
			//Priority:     "",
			//SameParty:    false,
			//SourceScheme: "",
			//SourcePort:   -1,
			//PartitionKey: "",
		})
	}

	return ret, nil
}
