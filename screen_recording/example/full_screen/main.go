package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"io/ioutil"
	"log"
	"os/exec"
	"time"
)

func main() {
	// 禁用chrome headless
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("window-position", "0,0"),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("window-size", "1920,1080"),
	)
	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	// create context
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	err := chromedp.Run(ctx, chromedp.Tasks{
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

	// Build the ffmpeg command.
	cmd := exec.Command("ffmpeg -video_size 1920x1080 -framerate 25 -f x11grab -i :0.0+0,00 -t 10 output.mp4")
	var stdErr bytes.Buffer
	var stdOut bytes.Buffer
	cmd.Stderr = &stdErr
	cmd.Stdout = &stdOut

	log.Printf("run 1")
	err = chromedp.Run(ctx, chromedp.Tasks{
		chromedp.Navigate("https://www.douyin.com/"),
		// 设置mask隐藏
		chromedp.Evaluate(`localStorage.setItem("douyin_web_hide_guide",1)`, nil),
		// 如果登录窗口出现，点击关闭
		chromedp.WaitVisible(`//*[@id="login-pannel"]`),
		chromedp.Click(`//*[@id="login-pannel"]//div[@class='dy-account-close']`),
		// 判断是否开始播放
		chromedp.WaitVisible(`//*[@id="sliderVideo"]//xg-controls/xg-inner-controls/xg-left-grid//xg-icon[@class="xgplayer-play"][@data-state="play"]`),
		// 开始录制
		chromedp.ActionFunc(func(ctx context.Context) error {
			err = cmd.Start()
			if err != nil {
				log.Fatalf("start err %v", err)
			}
			return err
		}),
		// 停止视频文件管道写入
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Printf("close succ")
			err := cmd.Wait()
			if err != nil {
				log.Printf("wait err %v : %v", err, stdErr.String())
				return err
			}
			log.Printf("cmd end")
			return nil
		}),
	})

	log.Printf("out is %v", stdOut.String())

	if err != nil {
		log.Fatalf("run err %v", err)
	}

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
