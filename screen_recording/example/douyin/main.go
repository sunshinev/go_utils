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

	i := 0
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch ev.(type) {
		case *page.EventScreencastFrame:
			go func() {
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
					_ = ioutil.WriteFile(fmt.Sprintf("image/video_shot_%v.jpeg", i), d, os.ModePerm)
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

			return nil
		}),
	})

	if err != nil {
		log.Fatalf("run _blank err %v", err)
	}

	err = chromedp.Run(ctx, chromedp.Tasks{
		chromedp.Navigate("https://www.douyin.com/"),
		// 设置mask隐藏
		chromedp.Evaluate(`localStorage.setItem("douyin_web_hide_guide",1)`, nil),
		// 如果登录窗口出现，点击关闭
		//chromedp.WaitVisible(`//*[@id="login-pannel"]`),
		//chromedp.Click(`//*[@id="login-pannel"]//div[@class='dy-account-close']`),
		// 判断是否开始播放
		chromedp.WaitVisible(`//*[@id="sliderVideo"]//xg-controls/xg-inner-controls/xg-left-grid//xg-icon[@class="xgplayer-play"][@data-state="play"]`),
		// 开始录制
		page.StartScreencast().WithFormat("jpeg"),
		// 5秒
		chromedp.Sleep(3 * time.Second),
		// 结束录制
		page.StopScreencast(),
		// todo 这个地方是否需要优化，为什么呢？
		chromedp.Sleep(1 * time.Second),
		// 停止视频文件管道写入
		chromedp.ActionFunc(func(ctx context.Context) error {
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
