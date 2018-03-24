package bots

import (
	"fmt"
	"github.com/line/line-bot-sdk-go/linebot"
	"github.com/mong0520/linebot-ptt/controllers"
	"github.com/mong0520/linebot-ptt/models"
	"gopkg.in/mgo.v2/bson"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
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
	DefaultTitle string = "💋表特看看"

	ActionNewest     string = "最新表特"
	ActionDailyHot   string = "📈 本日熱門"
	ActionMonthlyHot string = "🔥 近期熱門" //改成近期隨機, 先選出100個，然後隨機吐10筆
	ActionYearHot    string = "🏆 年度熱門"
	ActionRandom     string = "👩 隨機"
	ActionClick      string = "👉 點我打開"
	ActionHelp       string = "||| 選單"
	ActionAllImage   string = "打開圖片"

	ModeHttp      string = "http"
	ModeHttps     string = "https"
	ErrorNotFound string = "找不到關鍵字"
	AltText       string = "正妹只在手機上"
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
			meta.Log.Println(event.Postback.Data)
			postbackHandler(event)

		} else {
			meta.Log.Printf("got a %s event\n", event.Type)
		}
	}
}

func actionHandler(event *linebot.Event, action string, values url.Values) {
	switch action {
	case ActionNewest:
		actionNewest(event, values)
	case ActionAllImage:
		actionAllImage(event, values)
	case ActionDailyHot, ActionMonthlyHot, ActionYearHot, ActionRandom:
		actionGeneral(event, action, values)
	default:
		meta.Log.Println("Unimplement action handler", action)
	}
}

func actionGeneral(event *linebot.Event, action string, values url.Values) {
	meta.Log.Println("Enter actionGeneral, action = ", action)
	records := []models.ArticleDocument{}
	switch action {
	case ActionDailyHot:
		records, _ = controllers.GetMostLike(meta.Collection, maxCountOfCarousel, oneDayInSec)
	case ActionMonthlyHot:
		records, _ = controllers.GetMostLike(meta.Collection, maxCountOfCarousel, oneMonthInSec)
	case ActionYearHot:
		records, _ = controllers.GetMostLike(meta.Collection, maxCountOfCarousel, oneYearInSec)
	case ActionRandom:
		records, _ = controllers.GetRandom(meta.Collection, maxCountOfCarousel, "")
	default:
		return
	}
	template := getCarouseTemplate(records)
	if template != nil {
		sendCarouselMessage(event, template)
	}

}

func actionAllImage(event *linebot.Event, values url.Values) {
	if articleId := values.Get("article_id"); articleId != "" {
		template := getImgCarousTemplate(articleId)
		sendImgCarouseMessage(event, template)
	} else {
		meta.Log.Println("Unable to get article id", values)
	}
}

func actionNewest(event *linebot.Event, values url.Values) {
	columnCount := 9
	if currentPage, err := strconv.Atoi(values.Get("page")); err != nil {
		meta.Log.Println("Unable to parse parameters", values)
	} else {
		records, _ := controllers.Get(meta.Collection, currentPage, columnCount)
		template := getCarouseTemplate(records)

		if template == nil {
			meta.Log.Println("Unable to get template", values)
			return
		}

		// append next page column
		previousPage := currentPage - 1
		if previousPage < 0 {
			previousPage = 0
		}
		nextPage := currentPage + 1
		previousData := fmt.Sprintf("action=%s&page=%d", ActionNewest, previousPage)
		nextData := fmt.Sprintf("action=%s&page=%d", ActionNewest, nextPage)
		previousText := fmt.Sprintf("上一頁 %d", previousPage)
		nextText := fmt.Sprintf("下一頁 %d", nextPage)
		tmpColumn := linebot.NewCarouselColumn(
			defaultThumbnail,
			DefaultTitle,
			"繼續看？",
			linebot.NewMessageTemplateAction(ActionHelp, ActionHelp),
			linebot.NewPostbackTemplateAction(previousText, previousData, "", ""),
			linebot.NewPostbackTemplateAction(nextText, nextData, "", ""),
		)
		template.Columns = append(template.Columns, tmpColumn)

		sendCarouselMessage(event, template)
	}
}

func getCarouseTemplate(records []models.ArticleDocument) (template *linebot.CarouselTemplate) {
	if len(records) == 0 {
		return nil
	}

	columnList := []*linebot.CarouselColumn{}
	for _, result := range records {
		thumnailUrl := defaultImage
		imgUrlCounts := len(result.ImageLinks)
		lable := fmt.Sprintf("%s (%d)",ActionAllImage,  imgUrlCounts)
		title := result.ArticleTitle
		postBackData := fmt.Sprintf("action=%s&article_id=%s", ActionAllImage, result.ArticleID)
		text := fmt.Sprintf("%d 😍\t%d 😡", result.MessageCount.Push, result.MessageCount.Boo)

		if imgUrlCounts > 0 {
			thumnailUrl = result.ImageLinks[0]
		}
		if len(title) >= 40 {
			title = title[0:39]
		}
		//meta.Log.Println("===============", idx)
		//meta.Log.Println("Thumbnail Url = ", thumnailUrl)
		//meta.Log.Println("Title = ", title)
		//meta.Log.Println("Text = ", text)
		//meta.Log.Println("URL = ", result.URL)
		//meta.Log.Println("===============", idx)
		dataRandom := fmt.Sprintf("action=%s", ActionRandom)
		tmpColumn := linebot.NewCarouselColumn(
			thumnailUrl,
			title,
			text,
			linebot.NewURITemplateAction(ActionClick, result.URL),
			linebot.NewPostbackTemplateAction(ActionRandom, dataRandom, "", ""),
			linebot.NewPostbackTemplateAction(lable, postBackData, "", ""),
		)
		columnList = append(columnList, tmpColumn)
	}
	template = linebot.NewCarouselTemplate(columnList...)
	return template
}

func postbackHandler(event *linebot.Event) {
	m, _ := url.ParseQuery(event.Postback.Data)
	action := m.Get("action")
	meta.Log.Println("Action = ", action)
	actionHandler(event, action, m)
}

func getUserNameById(userId string) (userDisplayName string) {
	res, err := bot.GetProfile(userId).Do()
	if err != nil {
		userDisplayName = "Unknown"
	} else {
		userDisplayName = res.DisplayName
	}
	return userDisplayName
}

func textHander(event *linebot.Event, message string) {
	switch message {
	case ActionHelp:
		template := getMenuButtonTemplate(DefaultTitle)
		sendButtonMessage(event, template)
	default:
		records, _ := controllers.GetRandom(meta.Collection, maxCountOfCarousel, message)
		if records != nil && len(records) > 0 {
			template := getCarouseTemplate(records)
			sendCarouselMessage(event, template)
		} else {
			template := getMenuButtonTemplate(DefaultTitle)
			sendButtonMessage(event, template)
		}
	}
}

func getMenuButtonTemplate(title string) (template *linebot.ButtonsTemplate) {
	dataDaily := fmt.Sprintf("action=%s", ActionDailyHot)
	dataNewlest := fmt.Sprintf("action=%s&page=0", ActionNewest)
	dataYear := fmt.Sprintf("action=%s", ActionYearHot)
	dataRandom := fmt.Sprintf("action=%s", ActionRandom)
	template = linebot.NewButtonsTemplate(defaultThumbnail, title, "你可以試試看以下選項，或直接輸入關鍵字查詢",
		linebot.NewPostbackTemplateAction(ActionNewest, dataNewlest, "", ""),
		linebot.NewPostbackTemplateAction(ActionDailyHot, dataDaily, "", ""),
		linebot.NewPostbackTemplateAction(ActionYearHot, dataYear, "", ""),
		linebot.NewPostbackTemplateAction(ActionRandom, dataRandom, "", ""),
	)
	return template
}

func sendTextMessage(event *linebot.Event, text string) {
	if _, err := bot.ReplyMessage(event.ReplyToken, linebot.NewTextMessage(text)).Do(); err != nil {
		log.Println("Send Fail")
	}
}

func getImgCarousTemplate(articleId string) (template *linebot.ImageCarouselTemplate) {
	query := bson.M{"article_id": articleId}
	result, _ := controllers.GetOne(meta.Collection, query)
	urls := result.ImageLinks
	columnList := []*linebot.ImageCarouselColumn{}
	if len(urls) > 10 {
		urls = urls[0:10]
	}
	for _, url := range urls {
		tmpColumn := linebot.NewImageCarouselColumn(
			url,
			//linebot.NewURITemplateAction(ActionClick, url),
			linebot.NewURITemplateAction(ActionClick, result.URL),
		)
		columnList = append(columnList, tmpColumn)
	}
	template = linebot.NewImageCarouselTemplate(columnList...)
	return template
}

//
//func buildCarouseTemplate(action string, pagination bool, currentPage int) (template *linebot.CarouselTemplate) {
//	results := []models.ArticleDocument{}
//	switch action {
//	case ActionDailyHot:
//		results, _ = controllers.GetMostLike(meta.Collection, maxCountOfCarousel, oneDayInSec)
//	case ActionMonthlyHot:
//		results, _ = controllers.GetMostLike(meta.Collection, maxCountOfCarousel, oneMonthInSec)
//	case ActionYearHot:
//		results, _ = controllers.GetMostLike(meta.Collection, maxCountOfCarousel, oneYearInSec)
//	case ActionRandom:
//		results, _ = controllers.GetRandom(meta.Collection, maxCountOfCarousel, "")
//	case ActionNewest:
//		results, _ = controllers.Get(meta.Collection, currentPage, 9)
//	default:
//		results, _ = controllers.GetRandom(meta.Collection, maxCountOfCarousel, action)
//	}
//
//	columnList := []*linebot.CarouselColumn{}
//	meta.Log.Println("Found Records: ", len(results))
//	if len(results) == 0 {
//		return nil
//	}
//	for _, result := range results {
//		//meta.Log.Printf("%+v", result)
//		thumnailUrl := defaultThumbnail
//		imgUrlCounts := len(result.ImageLinks)
//		if imgUrlCounts > 0{
//			thumnailUrl = result.ImageLinks[0]
//		}
//		lable := fmt.Sprintf("所有圖片 (%d)", imgUrlCounts)
//		title := result.ArticleTitle
//		postBackData := fmt.Sprintf("action=%s&article_id=%s", ActionAllImage, result.ArticleID)
//		text := fmt.Sprintf("%d 😍\t%d 😡", result.MessageCount.Push, result.MessageCount.Boo)
//		if len(title) >= 40 {
//			title = title[0:39]
//		}
//		//meta.Log.Println("===============", idx)
//		//meta.Log.Println("Thumbnail Url = ", thumnailUrl)
//		//meta.Log.Println("Title = ", title)
//		//meta.Log.Println("Text = ", text)
//		//meta.Log.Println("URL = ", result.URL)
//		//meta.Log.Println("===============", idx)
//		tmpColumn := linebot.NewCarouselColumn(
//			thumnailUrl,
//			title,
//			text,
//			linebot.NewURITemplateAction(ActionClick, result.URL),
//			linebot.NewMessageTemplateAction(ActionRandom, ActionRandom),
//			linebot.NewPostbackTemplateAction(lable, postBackData, "", ""),
//			//linebot.NewMessageTemplateAction(ActionHelp, ActionHelp),
//		)
//		columnList = append(columnList, tmpColumn)
//	}
//
//	if pagination{
//		previousPage := currentPage - 1
//		if previousPage < 0{
//			previousPage = 0
//		}
//		nextPage := currentPage + 1
//		previousData := fmt.Sprintf("action=%s&page=%d", ActionNewest, previousPage)
//		nextData := fmt.Sprintf("action=%s&page=%d", ActionNewest, nextPage)
//		previousText := fmt.Sprintf("上一頁 %d", previousPage)
//		nextText := fmt.Sprintf("下一頁 %d", nextPage)
//		tmpColumn := linebot.NewCarouselColumn(
//			defaultImage,
//			"表特看看",
//			"繼續看？",
//			linebot.NewPostbackTemplateAction(previousText, previousData, "", ""),
//			linebot.NewPostbackTemplateAction(previousText, previousData, "", ""),
//			linebot.NewPostbackTemplateAction(nextText, nextData, "", ""),
//		)
//		if len(columnList) >= 10{
//			columnList[len(columnList)-1] = tmpColumn
//		} else{
//			columnList = append(columnList, tmpColumn)
//		}
//	}
//
//	template = linebot.NewCarouselTemplate(columnList...)
//
//	return template
//}

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

func sendImgCarouseMessage(event *linebot.Event, template *linebot.ImageCarouselTemplate) {
	if _, err := bot.ReplyMessage(event.ReplyToken, linebot.NewTemplateMessage("Carousel alt text", template)).Do(); err != nil {
		meta.Log.Println(err)
	}
}
