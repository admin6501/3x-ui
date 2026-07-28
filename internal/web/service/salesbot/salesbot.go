// Package salesbot runs the shop bot: a second Telegram bot, separate from the
// panel's notification bot, that sells configs to end users on a prepaid
// wallet. It has its own token and its own admin list so a shop can be handed
// out without exposing the panel's notification bot.
//
// It does not sell reseller panels — those are created by an admin from the
// panel's Admins page. Everything that moves money or grants access goes
// through service.ShopService, the same code path the panel UI uses; the bot is
// a front end, not a second source of truth.
package salesbot

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpproxy"
)

// Bot is the running sales bot. Exactly one is alive at a time; Reconcile
// starts, stops or restarts it to match the current settings.
type Bot struct {
	settingService service.SettingService
	shopService    service.ShopService
	inboundService *service.InboundService
	xrayService    *service.XrayService

	mu       sync.Mutex
	api      *telego.Bot
	cancel   context.CancelFunc
	handler  *th.BotHandler
	wg       sync.WaitGroup
	running  bool
	adminIds []int64
	// token and admins the running instance was started with, so Reconcile can
	// tell a settings change that matters from one that does not.
	startedToken  string
	startedAdmins string

	states *stateStore
}

var (
	instance   *Bot
	instanceMu sync.Mutex
)

// Manager returns the process-wide sales bot, creating it on first use.
func Manager(inboundSvc *service.InboundService, xraySvc *service.XrayService) *Bot {
	instanceMu.Lock()
	defer instanceMu.Unlock()
	if instance == nil {
		instance = &Bot{states: newStateStore()}
	}
	if inboundSvc != nil {
		instance.inboundService = inboundSvc
	}
	if xraySvc != nil {
		instance.xrayService = xraySvc
	}
	return instance
}

// IsRunning reports whether the long-polling loop is alive.
func (b *Bot) IsRunning() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.running
}

// Reconcile brings the bot in line with the settings: start it if it should be
// running, stop it if not, restart it when the token or admin list changed.
// Safe to call on every settings save.
func (b *Bot) Reconcile() {
	enabled, _ := b.settingService.GetSalesBotEnable()
	token, _ := b.settingService.GetSalesBotToken()
	admins, _ := b.settingService.GetSalesBotAdmins()
	token = strings.TrimSpace(token)

	if !enabled || token == "" {
		if b.IsRunning() {
			logger.Info("sales bot: disabled, stopping")
			b.Stop()
		}
		return
	}

	b.mu.Lock()
	sameConfig := b.running && b.startedToken == token && b.startedAdmins == admins
	b.mu.Unlock()
	if sameConfig {
		return
	}
	if b.IsRunning() {
		b.Stop()
	}
	if err := b.Start(); err != nil {
		logger.Warning("sales bot: start failed:", err)
	}
}

// Start brings the bot up with whatever the settings currently say.
func (b *Bot) Start() error {
	enabled, err := b.settingService.GetSalesBotEnable()
	if err != nil || !enabled {
		return nil
	}
	token, err := b.settingService.GetSalesBotToken()
	if err != nil || strings.TrimSpace(token) == "" {
		logger.Warning("sales bot: no token configured")
		return nil
	}
	adminsCSV, _ := b.settingService.GetSalesBotAdmins()

	api, err := b.newBotAPI(strings.TrimSpace(token))
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	updates, err := api.UpdatesViaLongPolling(ctx, &telego.GetUpdatesParams{Timeout: 20})
	if err != nil {
		cancel()
		return err
	}
	handler, err := th.NewBotHandler(api, updates)
	if err != nil {
		cancel()
		return err
	}

	b.mu.Lock()
	b.api = api
	b.cancel = cancel
	b.handler = handler
	b.running = true
	b.adminIds = parseAdminIds(adminsCSV)
	b.startedToken = strings.TrimSpace(token)
	b.startedAdmins = adminsCSV
	b.mu.Unlock()

	b.registerHandlers(handler)

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		if err := handler.Start(); err != nil {
			logger.Warning("sales bot: handler stopped:", err)
		}
	}()
	logger.Info("sales bot: started")
	return nil
}

// Stop shuts the bot down and waits for the polling loop to finish.
func (b *Bot) Stop() {
	b.mu.Lock()
	cancel := b.cancel
	handler := b.handler
	b.cancel = nil
	b.handler = nil
	b.running = false
	b.startedToken = ""
	b.startedAdmins = ""
	b.mu.Unlock()

	if handler != nil {
		handler.Stop()
	}
	if cancel != nil {
		cancel()
	}
	b.wg.Wait()
	b.states.reset()
	logger.Info("sales bot: stopped")
}

// newBotAPI builds the telego client, routing through the panel's Telegram
// proxy when one is set — the same egress the notification bot uses, since a
// server that needs a proxy to reach Telegram needs it for both bots.
func (b *Bot) newBotAPI(token string) (*telego.Bot, error) {
	proxyURL, _ := b.settingService.GetTgBotProxy()
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		proxyURL = b.settingService.PanelEgressProxyURL()
	}

	client := &fasthttp.Client{
		ReadTimeout:               30 * time.Second,
		WriteTimeout:              30 * time.Second,
		MaxIdleConnDuration:       60 * time.Second,
		MaxIdemponentCallAttempts: 3,
		MaxConnsPerHost:           50,
		MaxConnWaitTimeout:        10 * time.Second,
	}
	switch {
	case strings.HasPrefix(proxyURL, "socks5://"):
		client.Dial = fasthttpproxy.FasthttpSocksDialer(proxyURL)
	case strings.HasPrefix(proxyURL, "http://"), strings.HasPrefix(proxyURL, "https://"):
		client.Dial = fasthttpproxy.FasthttpHTTPDialer(proxyURL)
	}

	opts := []telego.BotOption{telego.WithFastHTTPClient(client)}
	if apiServer, _ := b.settingService.GetTgBotAPIServer(); strings.TrimSpace(apiServer) != "" {
		if safe, err := service.SanitizePublicHTTPURL(apiServer, false); err == nil {
			opts = append(opts, telego.WithAPIServer(safe))
		}
	}
	return telego.NewBot(token, opts...)
}

func parseAdminIds(csv string) []int64 {
	out := make([]int64, 0)
	for _, part := range strings.Split(csv, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			logger.Warning("sales bot: ignoring unparsable admin id", part)
			continue
		}
		out = append(out, id)
	}
	return out
}

// isAdmin reports whether a Telegram user may use the bot's admin side.
func (b *Bot) isAdmin(id int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, adminId := range b.adminIds {
		if adminId == id {
			return true
		}
	}
	return false
}

func (b *Bot) admins() []int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]int64, len(b.adminIds))
	copy(out, b.adminIds)
	return out
}

// ---------------------------------------------------------------- sending --

func (b *Bot) client() *telego.Bot {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.running {
		return nil
	}
	return b.api
}

// send posts a message, chunking anything over Telegram's limit.
func (b *Bot) send(chatId int64, text string, markup ...telego.ReplyMarkup) {
	api := b.client()
	if api == nil || strings.TrimSpace(text) == "" {
		return
	}
	chunks := splitForTelegram(text)
	for i, chunk := range chunks {
		params := &telego.SendMessageParams{
			ChatID:    tu.ID(chatId),
			Text:      chunk,
			ParseMode: telego.ModeHTML,
		}
		if len(markup) > 0 && i == len(chunks)-1 {
			params.ReplyMarkup = markup[0]
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		_, err := api.SendMessage(ctx, params)
		cancel()
		if err != nil {
			logger.Warning("sales bot: send failed:", err)
			return
		}
	}
}

// sendPhoto forwards a photo already stored on Telegram (a payment receipt) by
// its file id, so the bot never has to download or store the image itself.
func (b *Bot) sendPhoto(chatId int64, fileId, caption string, markup telego.ReplyMarkup) {
	api := b.client()
	if api == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := api.SendPhoto(ctx, &telego.SendPhotoParams{
		ChatID:      tu.ID(chatId),
		Photo:       telego.InputFile{FileID: fileId},
		Caption:     caption,
		ParseMode:   telego.ModeHTML,
		ReplyMarkup: markup,
	})
	if err != nil {
		logger.Warning("sales bot: send photo failed, falling back to text:", err)
		b.send(chatId, caption, markup)
	}
}

// answer closes the spinner on an inline button.
func (b *Bot) answer(queryId, text string) {
	api := b.client()
	if api == nil {
		return
	}
	_ = api.AnswerCallbackQuery(context.Background(), &telego.AnswerCallbackQueryParams{
		CallbackQueryID: queryId,
		Text:            text,
	})
}

// notifyAdmins pushes a message to every configured admin.
func (b *Bot) notifyAdmins(text string, markup ...telego.ReplyMarkup) {
	for _, id := range b.admins() {
		b.send(id, text, markup...)
	}
}

// splitForTelegram breaks a long message on paragraph boundaries, falling back
// to a hard cut for a single paragraph that is itself too long.
func splitForTelegram(text string) []string {
	const limit = 3500
	if len(text) <= limit {
		return []string{text}
	}
	var out []string
	current := ""
	for _, para := range strings.Split(text, "\n\n") {
		if len(para) > limit {
			if current != "" {
				out = append(out, current)
				current = ""
			}
			for len(para) > limit {
				out = append(out, para[:limit])
				para = para[limit:]
			}
		}
		if current == "" {
			current = para
			continue
		}
		if len(current)+len(para)+2 > limit {
			out = append(out, current)
			current = para
			continue
		}
		current += "\n\n" + para
	}
	if strings.TrimSpace(current) != "" {
		out = append(out, current)
	}
	return out
}

// ----------------------------------------------------------- conversations --

// state is one in-progress multi-step conversation.
type state struct {
	step string
	// orderId is the top-up the conversation is about.
	orderId int
	// targetUser is the shop user an admin action applies to.
	targetUser int64
	// configId is the config a buyer's config-management step applies to.
	configId int
	touched  time.Time
}

// stateStore keeps per-chat conversation state. Handlers run on the dispatch
// goroutine and on worker goroutines, so it has to be safe for concurrent use;
// it also expires abandoned conversations rather than growing forever.
type stateStore struct {
	mu sync.Mutex
	m  map[int64]*state
}

func newStateStore() *stateStore { return &stateStore{m: map[int64]*state{}} }

func (s *stateStore) get(chatId int64) (*state, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.m[chatId]
	if !ok {
		return nil, false
	}
	copied := *st
	return &copied, true
}

func (s *stateStore) set(chatId int64, st *state) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st.touched = time.Now()
	s.m[chatId] = st
	// Opportunistic cleanup: a user who starts a flow and goes silent should
	// not hold an entry forever.
	for id, other := range s.m {
		if time.Since(other.touched) > time.Hour {
			delete(s.m, id)
		}
	}
}

func (s *stateStore) clear(chatId int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, chatId)
}

func (s *stateStore) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m = map[int64]*state{}
}
