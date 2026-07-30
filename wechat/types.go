package wechat

import (
	"github.com/SpellingDragon/wechat-robot-go/wechat/internal/media"
	"github.com/SpellingDragon/wechat-robot-go/wechat/internal/middleware"
	"github.com/SpellingDragon/wechat-robot-go/wechat/internal/model"
	"github.com/SpellingDragon/wechat-robot-go/wechat/internal/store"
	"github.com/SpellingDragon/wechat-robot-go/wechat/internal/text"
)

// --- Message types from model package ---

// Message represents a WeChat message from the iLink API.
type Message = model.Message

// MessageItem represents a single content item in a message.
type MessageItem = model.MessageItem

// TextItem represents text content.
type TextItem = model.TextItem

// ImageItem represents image content with CDN reference.
type ImageItem = model.ImageItem

// VoiceItem represents voice content.
type VoiceItem = model.VoiceItem

// FileItem represents a file attachment.
type FileItem = model.FileItem

// VideoItem represents video content.
type VideoItem = model.VideoItem

// CDNMedia represents CDN media reference with encryption info.
type CDNMedia = model.CDNMedia

// BaseInfo carries channel version metadata.
type BaseInfo = model.BaseInfo

// MessageType represents the type of message sender.
type MessageType = model.MessageType

// MessageState represents the state of a message.
type MessageState = model.MessageState

// ItemType represents the type of message content.
type ItemType = model.ItemType

// MessageHandler is the callback type for handling incoming messages.
type MessageHandler = model.MessageHandler

// GetUpdatesRequest is the request body for POST /ilink/bot/getupdates.
type GetUpdatesRequest = model.GetUpdatesRequest

// GetUpdatesResponse is the response from POST /ilink/bot/getupdates.
type GetUpdatesResponse = model.GetUpdatesResponse

// SendMessageRequest is the request body for POST /ilink/bot/sendmessage.
type SendMessageRequest = model.SendMessageRequest

// SendMessageResponse is the response from POST /ilink/bot/sendmessage.
type SendMessageResponse = model.SendMessageResponse

// --- Constants from model package ---
// Constants cannot be aliased, must be redeclared.

const (
	// MessageType constants
	MessageTypeNone = model.MessageTypeNone
	MessageTypeUser = model.MessageTypeUser
	MessageTypeBot  = model.MessageTypeBot

	// MessageState constants
	MessageStateNew        = model.MessageStateNew
	MessageStateGenerating = model.MessageStateGenerating
	MessageStateFinish     = model.MessageStateFinish

	// ItemType constants
	ItemTypeText  = model.ItemTypeText
	ItemTypeImage = model.ItemTypeImage
	ItemTypeVoice = model.ItemTypeVoice
	ItemTypeFile  = model.ItemTypeFile
	ItemTypeVideo = model.ItemTypeVideo
)

// --- Store types ---

// ContextTokenData holds the persisted context token for a user conversation.
type ContextTokenData = store.ContextTokenData

// ContextTokenStore is the interface for persisting context tokens.
type ContextTokenStore = store.ContextTokenStore

// FileContextTokenStore implements ContextTokenStore by persisting tokens to JSON files.
type FileContextTokenStore = store.FileContextTokenStore

// MemoryContextTokenStore implements ContextTokenStore with in-memory storage only.
type MemoryContextTokenStore = store.MemoryContextTokenStore

// --- Store constructor wrappers ---

// NewFileContextTokenStore creates a new FileContextTokenStore.
var NewFileContextTokenStore = store.NewFileContextTokenStore

// NewMemoryContextTokenStore creates a new in-memory context token store.
var NewMemoryContextTokenStore = store.NewMemoryContextTokenStore

// --- Middleware types ---

// Middleware wraps a MessageHandler to add cross-cutting concerns.
type Middleware = middleware.Middleware

// --- Middleware function wrappers ---

// Chain composes multiple middlewares into a single Middleware.
var Chain = middleware.Chain

// WithRecovery returns a middleware that recovers from panics in the handler.
var WithRecovery = middleware.WithRecovery

// WithLogging returns a middleware that logs incoming messages and handler results.
var WithLogging = middleware.WithLogging

// --- Media types ---

// MediaManager handles media file upload and download with AES encryption.
type MediaManager = media.MediaManager

// UploadResult contains the CDN reference information after a successful upload.
type UploadResult = media.UploadResult

// UploadURLRequest is the request body for POST /ilink/bot/getuploadurl.
type UploadURLRequest = media.UploadURLRequest

// UploadURLResponse is the response from POST /ilink/bot/getuploadurl.
type UploadURLResponse = media.UploadURLResponse

// DownloadOptions controls streaming download behavior.
// MaxSize is the maximum number of ciphertext bytes to accept; 0 means no limit.
type DownloadOptions = media.DownloadOptions

// ErrMaxSizeExceeded is returned when a streaming download exceeds
// DownloadOptions.MaxSize. Use errors.Is to detect it.
var ErrMaxSizeExceeded = media.ErrMaxSizeExceeded

// --- Text utilities ---

// SplitText splits a long text into multiple chunks that fit within the message limit.
var SplitText = text.SplitText

// DefaultMaxTextLength is the default maximum text length per message.
const DefaultMaxTextLength = text.DefaultMaxTextLength
