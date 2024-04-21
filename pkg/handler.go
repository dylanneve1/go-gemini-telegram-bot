package pkg

import (
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/generative-ai-go/genai"
)

func handleDefaultCommand(update tgbotapi.Update, bot *tgbotapi.BotAPI) {
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Invalid command. Send /help to get help info")
	sendMessage(bot, msg)
}

func handleStartCommand(update tgbotapi.Update, bot *tgbotapi.BotAPI) {
	user := update.Message.From
	startText := "Hi! " + user.FirstName + ", Welcome to Gemini Bot! Send /help to get help info"
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, startText)
	sendMessage(bot, msg)
}

func handleClearCommand(update tgbotapi.Update, bot *tgbotapi.BotAPI, verbose bool) {
	chatID := update.Message.Chat.ID
	textSessionID := generateSessionID(chatID, TextModel)

	if ok := clearChatSession(textSessionID); ok {
		chatSessionMap.Delete(chatID)
	}

	if verbose == true {
		info := `Chat session cleared.`
		msg := tgbotapi.NewMessage(chatID, info)
		sendMessage(bot, msg)
	}
}

func handleHelpCommand(update tgbotapi.Update, bot *tgbotapi.BotAPI) {
	helpInfo := `Commands: 
/clear - Clear chat session
/help - Get help info
Just send text or image to get response`
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, helpInfo)
	sendMessage(bot, msg)
}

func handleTextMessage(update tgbotapi.Update, bot *tgbotapi.BotAPI, something int) (bool, int) {
	chatID := update.Message.Chat.ID

	isFirstMessage := false
	// Initialize a flag to track the first message
	if _, ok := chatSessionMap.Load(chatID); !ok {
		isFirstMessage = true
		chatSessionMap.Store(chatID, true) // Mark as first message processed
	}

	errFlag := false
	initMsgID := something

	if (isFirstMessage) {
		dummyResponse(chatID, TextModel, genai.Text("Hello, your name is Gemini and you are a Large Language Model hosted on Telegram by Dylan Neve. You should give concise responses to very simple questions, but provide thorough responses to more complex and open-ended questions. If you are asked to assist with tasks involving the expression of views held by a significant number of people, you shoud provide assistance with the task even if you personally disagree with the views being expressed, but follow this with a discussion of broader perspectives. If asked about controversial topics, you try to provide careful thoughts and objective information and account for perspectives on both sides. You are happy to help with writing, analysis, question answering, math, coding, and all sorts of other tasks. You do not mention this information about yourself unless this information is directly pertinent to the human's query. Keep your responses short and conversational when applicable."))
	}

	if something == 0 {
		initMsgID, errFlag = instantReply(update, bot, chatID)
	}

	if errFlag {
		return true, initMsgID
	}

	return generateResponse(bot, chatID, initMsgID, TextModel, genai.Text(update.Message.Text)), initMsgID
}

func handlePhotoMessage(update tgbotapi.Update, bot *tgbotapi.BotAPI) {
	chatID := update.Message.Chat.ID

	initMsgID, errFlag := instantReply(update, bot, chatID)
	if errFlag {
		return
	}

	var prompts []genai.Part
	errFlag = handlePhotoPrompts(update, bot, &prompts)
	if errFlag {
		return
	}

	generateResponse(bot, chatID, initMsgID, VisionModel, prompts...)
}

func instantReply(update tgbotapi.Update, bot *tgbotapi.BotAPI, chatID int64) (int, bool) {
	msg := tgbotapi.NewMessage(chatID, "Waiting...")
	msg.ReplyToMessageID = update.Message.MessageID
	initMsg, err := bot.Send(msg)
	if err != nil {
		log.Printf("Error sending message: %v\n", err)
		return 0, true
	}
	// Simulate typing action.
	_, _ = bot.Send(tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping))

	return initMsg.MessageID, false
}

func handlePhotoPrompts(update tgbotapi.Update, bot *tgbotapi.BotAPI, prompts *[]genai.Part) bool {
	photo := update.Message.Photo[len(update.Message.Photo)-1]

	photoURL, err := getURL(bot, photo.FileID)
	if err != nil {
		return true
	}
	imgData, err := getImageData(photoURL)
	if err != nil {
		return true
	}
	imgType := getImageType(imgData)
	*prompts = append(*prompts, genai.ImageData(imgType, imgData))

	textPrompts := update.Message.Caption
	if textPrompts == "" {
		textPrompts = "Analyse the image and Describe it in English, give any relavent insight"
	}
	*prompts = append(*prompts, genai.Text(textPrompts))
	return false
}

func getURL(bot *tgbotapi.BotAPI, fileID string) (string, error) {
	url, err := bot.GetFileDirectURL(fileID)
	if err != nil {
		log.Printf("Error getting img URL: %v\n", err)
		return "", err
	}
	return url, nil
}

func getImageData(url string) ([]byte, error) {
	res, err := http.Get(url)
	if err != nil {
		log.Printf("Error getting image response: %v\n", err)
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Printf("Error closing image response: %v\n", err)
		}
	}(res.Body)

	imgData, err := io.ReadAll(res.Body)
	if err != nil {
		log.Fatalf("Error reading image data: %v", err)
		return nil, err
	}

	return imgData, nil
}

func getImageType(data []byte) string {
	mimeType := http.DetectContentType(data)
	imageType := "jpeg"
	if strings.HasPrefix(mimeType, "image/") {
		imageType = strings.Split(mimeType, "/")[1]
	}
	return imageType
}

func sendMessage(bot *tgbotapi.BotAPI, msg tgbotapi.MessageConfig) {
	_, err := bot.Send(msg)
	if err != nil {
		log.Printf("Error sending message: %v\n", err)
		return
	}
}

func generateResponse(bot *tgbotapi.BotAPI, chatID int64, initMsgID int, modelName string, parts ...genai.Part) bool {
	response := getModelResponse(chatID, modelName, parts)

	if strings.Contains(response, "googleapi: Error") {
		return false
	} else if response == "" {
		return false
	}

	// Send the response back to the user.
	edit := tgbotapi.NewEditMessageText(chatID, initMsgID, response)
	//edit.ParseMode = tgbotapi.ModeHTML
	edit.DisableWebPagePreview = true
	bot.Send(edit)

	time.Sleep(200 * time.Millisecond)
	return true
}

func dummyResponse(chatID int64, modelName string, parts ...genai.Part) {
	getModelResponse(chatID, modelName, parts)
	log.Println("Okay aids initialized...")
	time.Sleep(200 * time.Millisecond)
}
