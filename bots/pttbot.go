package bots

import (
	"fmt"
	"github.com/line/line-bot-sdk-go/linebot"
	"github.com/mong0520/linebot-ptt/controllers"
	"github.com/mong0520/linebot-ptt/models"
	"log"
	"mvdan.cc/xurls"
	"net/http"
	"os"
	"strings"
)

var bot *linebot.Client
var meta *models.Model
var maxCountOfCarousel = 10
var defaultImage = "https://i.imgur.com/WAnWk7K.png"
var defaultThumbnail = "https://i.imgur.com/StcRAPB.png"
var oneDayInSec = 60 * 60 * 24
var oneMonthInSec = oneDayInSec * 30
var oneYearInSec = oneMonthInSec * 365
var SSLCertPath = "/etc/dehydrated/certs/nt1.me/fullchain.pem"
var SSLPrivateKeyPath = "/etc/dehydrated/certs/nt1.me/privkey.pem"

// EventType constants
const (
	DefaultTitle	 string = "💋表特看看"
	ActionDailyHot   string = "📈 本日熱門"
	ActionMonthlyHot string = "🔥 近期熱門" //改成近期隨機, 先選出100個，然後隨機吐10筆
	ActionYearHot    string = "🏆 年度熱門"
	ActionRandom     string = "👩 隨機"
	ActionClick      string = "👉 點我打開"
	ActionHelp       string = "||| 選單"
	ModeHttp         string = "http"
	ModeHttps        string = "https"
	ErrorNotFound    string = "找不到關鍵字"
	AltText 		 string = "正妹只在手機上"
)

func InitLineBot(m *models.Model) {
	var err error
	meta = m
	secret := os.Getenv("ChannelSecret")
	token := os.Getenv("ChannelAccessToken")
	bot, err = linebot.New(secret, token)
	if err != nil {
		log.Println(err)
	}
	//log.Println("Bot:", bot, " err:", err)
	http.HandleFunc("/callback", callbackHandler)
	port := os.Getenv("PORT")
	//port := "8080"
	addr := fmt.Sprintf(":%s", port)
	runMode := os.Getenv("RUNMODE")
	m.Log.Printf("Run Mode = %s\n", runMode)
	if strings.ToLower(runMode) == ModeHttps {
		m.Log.Printf("Secure listen on %s with \n", addr)
		http.ListenAndServeTLS(addr, SSLCertPath, SSLPrivateKeyPath, nil)
	} else {
		m.Log.Printf("Listen on %s\n", addr)
		http.ListenAndServe(addr, nil)
	}
}

func callbackHandler(w http.ResponseWriter, r *http.Request) {
	events, err := bot.ParseRequest(r)

	if err != nil {
		if err == linebot.ErrInvalidSignature {
			w.WriteHeader(400)
		} else {
			w.WriteHeader(500)
		}
		return
	}

	for _, event := range events {
		if event.Type == linebot.EventTypeMessage {
			userDisplayName := getUserNameById(event.Source.UserID)
			meta.Log.Printf("Receieve Event Type = %s from User [%s](%s), or Room [%s] or Group [%s]\n",
				event.Type, userDisplayName, event.Source.UserID, event.Source.RoomID, event.Source.GroupID)

			switch message := event.Message.(type) {
			case *linebot.TextMessage:
				meta.Log.Println("Text = ", message.Text)
				textHander(event, message.Text)
			default:
				meta.Log.Println("Unimplemented handler for event type ", event.Type)
			}
		} else if event.Type == linebot.EventTypePostback {
			meta.Log.Println("got a postback event")
		} else {
			meta.Log.Printf("got a %s event\n", event.Type)
		}
	}
}

func getUserNameById(userId string)(userDisplayName string){
	res, err := bot.GetProfile(userId).Do()
	if err != nil {
		userDisplayName = "Unknown"
	} else {
		userDisplayName = res.DisplayName
	}
}

func textHander(event *linebot.Event, message string) {
	switch message {
	case ActionDailyHot, ActionMonthlyHot, ActionYearHot, ActionRandom:
		if template := buildCarouseTemplate(message); template != nil {
			sendCarouselMessage(event, template)
		} else {
			template := buildButtonTemplate(ErrorNotFound)
			sendButtonMessage(event, template)
		}
	case ActionHelp:
		template := buildButtonTemplate(DefaultTitle)
		sendButtonMessage(event, template)
	default:
		// receieve an undefined text and event is from a user
		if event.Source.UserID != "" && event.Source.GroupID == "" && event.Source.RoomID == ""{
			if template := buildCarouseTemplate(message); template != nil {
				sendCarouselMessage(event, template)
			} else {
				template := buildButtonTemplate(ErrorNotFound)
				sendButtonMessage(event, template)
			}
		}
	}
}

func buildButtonTemplate(title string) (template *linebot.ButtonsTemplate) {
	template = linebot.NewButtonsTemplate(defaultThumbnail, title, "你可以試試看以下選項，或直接輸入關鍵字查詢",
		linebot.NewMessageTemplateAction(ActionDailyHot, ActionDailyHot),
		linebot.NewMessageTemplateAction(ActionMonthlyHot, ActionMonthlyHot),
		linebot.NewMessageTemplateAction(ActionYearHot, ActionYearHot),
		linebot.NewMessageTemplateAction(ActionRandom, ActionRandom),
	)
	return template
}

func sendTextMessage(event *linebot.Event, text string) {
	if _, err := bot.ReplyMessage(event.ReplyToken, linebot.NewTextMessage(text)).Do(); err != nil {
		log.Println("Send Fail")
	}
}

func findImageInContent(content string) (img string) {
	imgs := xurls.Relaxed().FindAllString(content, -1)
	if imgs != nil {
		for _, img := range imgs {
			if strings.HasSuffix(strings.ToLower(img), "jpg") {
				img = strings.Replace(img, "http://", "https://", -1)
				return img
			}
		}
		//meta.Log.Println("try to append jpg in the end")
		img := imgs[0] + ".jpg"
		img = strings.Replace(img, "http://", "https://", -1)
		return img
	} else {
		return defaultImage
	}

}

func buildCarouseTemplate(action string) (template *linebot.CarouselTemplate) {
	results := []models.ArticleDocument{}
	switch action {
	case ActionDailyHot:
		results, _ = controllers.GetMostLike(meta.Collection, maxCountOfCarousel, oneDayInSec)
	case ActionMonthlyHot:
		results, _ = controllers.GetMostLike(meta.Collection, maxCountOfCarousel, oneMonthInSec)
	case ActionYearHot:
		results, _ = controllers.GetMostLike(meta.Collection, maxCountOfCarousel, oneYearInSec)
	case ActionRandom:
		results, _ = controllers.GetRandom(meta.Collection, maxCountOfCarousel, "")
	default:
		results, _ = controllers.GetRandom(meta.Collection, maxCountOfCarousel, action)
	}

	columnList := []*linebot.CarouselColumn{}
	meta.Log.Println("Found Records: ", len(results))
	if len(results) == 0 {
		return nil
	}
	for idx, result := range results {
		//meta.Log.Printf("%+v", result)
		//thumnailUrl := "https://c1.sd"
		thumnailUrl := findImageInContent(result.Content)
		title := result.ArticleTitle
		text := fmt.Sprintf("%d 😍\t%d 😡", result.MessageCount.Push, result.MessageCount.Boo)
		if len(title) >= 40 {
			title = title[0:39]
		}
		meta.Log.Println("===============", idx)
		meta.Log.Println("Thumbnail Url = ", thumnailUrl)
		meta.Log.Println("Title = ", title)
		meta.Log.Println("Text = ", text)
		meta.Log.Println("URL = ", result.URL)
		meta.Log.Println("===============", idx)
		tmpColumn := linebot.NewCarouselColumn(
			thumnailUrl,
			title,
			text,
			linebot.NewURITemplateAction(ActionClick, result.URL),
			linebot.NewMessageTemplateAction(ActionRandom, ActionRandom),
			linebot.NewMessageTemplateAction(ActionHelp, ActionHelp),
		)
		columnList = append(columnList, tmpColumn)
	}

	template = linebot.NewCarouselTemplate(columnList...)

	return template
}

func sendCarouselMessage(event *linebot.Event, template *linebot.CarouselTemplate) {
	if _, err := bot.ReplyMessage(event.ReplyToken, linebot.NewTemplateMessage(AltText, template)).Do(); err != nil {
		meta.Log.Println(err)
	}
}

func sendButtonMessage(event *linebot.Event, template *linebot.ButtonsTemplate) {
	if _, err := bot.ReplyMessage(event.ReplyToken, linebot.NewTemplateMessage(AltText, template)).Do(); err != nil {
		meta.Log.Println(err)
	}
}

//func sendImgCarouseMessage(event *linebot.Event, template *linebot.ImageCarouselTemplate) {
//	if _, err := bot.ReplyMessage(event.ReplyToken, linebot.NewTemplateMessage("Carousel alt text", template)).Do(); err != nil {
//		meta.Log.Println(err)
//	}
//}
