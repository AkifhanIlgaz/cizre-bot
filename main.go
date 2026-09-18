// Telegram Grup Mesajlarını Google Sheets'e Kaydeden Bot (Go)
//
// Kurulum:
//
//	go mod tidy
//
// Kullanmadan önce aşağıdaki değerleri kendi bilgilerinle değiştir:
//  1. botToken        -> @BotFather'dan aldığın token (env var ile de verebilirsin)
//  2. credentialsFile -> Google servis hesabı json dosyasının yolu
//  3. spreadsheetID   -> Google Sheets URL'indeki uzun ID
//     (https://docs.google.com/spreadsheets/d/BURASI/edit)
//  4. sheetName       -> içindeki sekme adı (örn. "Sayfa1")
//
// NOT: Botun grup mesajlarını görebilmesi için @BotFather üzerinden
//
//	/setprivacy -> Disable yapman gerekiyor.
//
// NOT: Google Sheets dosyasını, credentials.json içindeki
//
//	"client_email" adresiyle "Düzenleyen" olarak paylaşmayı unutma.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"golang.org/x/oauth2/google"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

var turkishTitleCaser = cases.Title(language.Turkish)

const (
	credentialsFile = "credentials.json"
	spreadsheetID   = "1J09zqMqG9_Ft1jHJHC6If4CZSyaWNpMTpyFazIFCLic"
	sheetName       = "Sayfa1"
)

func main() {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN env değişkeni ayarlanmalı")
	}

	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatalf("bot oluşturulamadı: %v", err)
	}
	log.Printf("Bot başlatıldı: @%s", bot.Self.UserName)

	sheetsService, err := newSheetsService(credentialsFile)
	if err != nil {
		log.Fatalf("google sheets bağlantısı kurulamadı: %v", err)
	}

	// Başlık satırını bir kere ekle (sayfa boşsa)
	ensureHeader(sheetsService)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	log.Println("Mesajlar dinleniyor...")
	for update := range updates {
		if update.Message == nil || update.Message.Text == "" {
			continue
		}
		go handleMessage(sheetsService, update.Message)
	}
}

func newSheetsService(credentialsFile string) (*sheets.Service, error) {
	ctx := context.Background()

	data, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("credentials dosyası okunamadı: %w", err)
	}

	config, err := google.JWTConfigFromJSON(data, sheets.SpreadsheetsScope)
	if err != nil {
		return nil, fmt.Errorf("jwt config oluşturulamadı: %w", err)
	}

	client := config.Client(ctx)
	return sheets.NewService(ctx, option.WithHTTPClient(client))
}

func ensureHeader(svc *sheets.Service) {
	rangeStr := fmt.Sprintf("%s!A1:D1", sheetName)
	resp, err := svc.Spreadsheets.Values.Get(spreadsheetID, rangeStr).Do()
	if err == nil && len(resp.Values) > 0 {
		return // zaten var
	}

	header := &sheets.ValueRange{
		Values: [][]interface{}{
			{"Tarih", "Saat", "Öğrenci", "Sayfa Sayısı"},
		},
	}
	_, err = svc.Spreadsheets.Values.Update(spreadsheetID, rangeStr, header).
		ValueInputOption("RAW").Do()
	if err != nil {
		log.Printf("başlık satırı eklenemedi: %v", err)
	}
}

// parseReadingMessage mesajın son kelimesini sayfa sayısı, öncesini
// öğrenci ismi olarak ayrıştırır. "Ahmet Yılmaz 25" -> ("Ahmet Yılmaz", 25, true)
func parseReadingMessage(text string) (studentName string, pages int, ok bool) {
	fields := strings.Fields(text)
	if len(fields) < 2 {
		return "", 0, false
	}

	last := fields[len(fields)-1]
	pages, err := strconv.Atoi(last)
	if err != nil || pages <= 0 {
		return "", 0, false
	}

	studentName = turkishTitleCaser.String(strings.Join(fields[:len(fields)-1], " "))
	return studentName, pages, true
}

func handleMessage(svc *sheets.Service, msg *tgbotapi.Message) {
	now := time.Now()

	var rowValues []interface{}
	if studentName, pages, ok := parseReadingMessage(msg.Text); ok {
		rowValues = []interface{}{
			now.Format("2006-01-02"),
			now.Format("15:04:05"),
			studentName,
			pages,
		}
	} else {
		// Format uymuyorsa ham mesajı Öğrenci sütununa, Sayfa Sayısı'nı boş bırak.
		rowValues = []interface{}{
			now.Format("2006-01-02"),
			now.Format("15:04:05"),
			msg.Text,
			"",
		}
	}

	row := &sheets.ValueRange{
		Values: [][]interface{}{rowValues},
	}

	rangeStr := fmt.Sprintf("%s!A:D", sheetName)
	_, err := svc.Spreadsheets.Values.Append(spreadsheetID, rangeStr, row).
		ValueInputOption("RAW").
		InsertDataOption("INSERT_ROWS").
		Do()
	if err != nil {
		log.Printf("satır eklenemedi: %v", err)
		return
	}

	log.Printf("kaydedildi: %.50s", msg.Text)
}
