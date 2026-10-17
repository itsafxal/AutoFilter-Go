package stream

import (
	"html/template"
	"io"
)

// PageData holds metadata passed to the stream/watch web template.
type PageData struct {
	Title         string
	FileName      string
	FileSizeStr   string
	FileSizeRaw   int64
	MimeType      string
	FileType      string
	IsVideo           bool
	IsAudio           bool
	IsMKV             bool
	IsBrowserPlayable bool
	StreamURL         string
	DownloadURL       string
	WatchURL          string
	PlaylistURL       string
	BotUsername       string
	VLCUrl            string
	MXPlayerUrl       string
	PlayItUrl         string
}

const watchTemplateHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{ .FileName }} - Fast Stream & Download</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Outfit:wght@400;500;600;700;800&family=JetBrains+Mono:wght@400;500&display=swap" rel="stylesheet">
    <style>
        :root {
            --bg-base: #07090e;
            --bg-card: rgba(18, 24, 38, 0.75);
            --bg-card-hover: rgba(26, 34, 52, 0.9);
            --border-glass: rgba(255, 255, 255, 0.08);
            --border-glass-glow: rgba(99, 102, 241, 0.35);
            --primary-gradient: linear-gradient(135deg, #6366f1 0%, #8b5cf6 50%, #ec4899 100%);
            --accent-cyan: #06b6d4;
            --accent-emerald: #10b981;
            --accent-purple: #8b5cf6;
            --text-main: #f8fafc;
            --text-muted: #94a3b8;
            --text-subtle: #64748b;
        }

        * {
            box-sizing: border-box;
            margin: 0;
            padding: 0;
        }

        body {
            font-family: 'Outfit', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background-color: var(--bg-base);
            background-image: 
                radial-gradient(circle at 15% 15%, rgba(99, 102, 241, 0.12) 0%, transparent 40%),
                radial-gradient(circle at 85% 85%, rgba(236, 72, 153, 0.08) 0%, transparent 40%),
                linear-gradient(rgba(255, 255, 255, 0.02) 1px, transparent 1px),
                linear-gradient(90deg, rgba(255, 255, 255, 0.02) 1px, transparent 1px);
            background-size: 100% 100%, 100% 100%, 40px 40px, 40px 40px;
            color: var(--text-main);
            min-height: 100vh;
            display: flex;
            flex-direction: column;
            align-items: center;
            overflow-x: hidden;
        }

        .header {
            width: 100%;
            max-width: 1200px;
            padding: 20px 24px;
            display: flex;
            justify-content: space-between;
            align-items: center;
            border-bottom: 1px solid var(--border-glass);
            backdrop-filter: blur(12px);
            -webkit-backdrop-filter: blur(12px);
        }

        .brand {
            display: flex;
            align-items: center;
            gap: 12px;
            text-decoration: none;
            color: var(--text-main);
        }

        .brand-badge {
            background: var(--primary-gradient);
            width: 38px;
            height: 38px;
            border-radius: 10px;
            display: flex;
            align-items: center;
            justify-content: center;
            font-weight: 800;
            font-size: 20px;
            box-shadow: 0 4px 14px rgba(99, 102, 241, 0.4);
        }

        .brand-text {
            font-weight: 700;
            font-size: 19px;
            letter-spacing: -0.5px;
            background: linear-gradient(135deg, #ffffff 30%, #94a3b8 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }

        .tg-btn {
            display: inline-flex;
            align-items: center;
            gap: 8px;
            padding: 8px 16px;
            background: rgba(36, 161, 222, 0.15);
            border: 1px solid rgba(36, 161, 222, 0.4);
            border-radius: 9999px;
            color: #38bdf8;
            text-decoration: none;
            font-size: 14px;
            font-weight: 600;
            transition: all 0.25s ease;
        }

        .tg-btn:hover {
            background: rgba(36, 161, 222, 0.25);
            transform: translateY(-1px);
            box-shadow: 0 4px 12px rgba(36, 161, 222, 0.3);
        }

        .main-container {
            width: 100%;
            max-width: 1060px;
            padding: 32px 20px 60px;
            display: flex;
            flex-direction: column;
            gap: 28px;
        }

        /* Player Container */
        .player-card {
            background: var(--bg-card);
            border: 1px solid var(--border-glass);
            border-radius: 20px;
            overflow: hidden;
            box-shadow: 0 20px 40px rgba(0, 0, 0, 0.6);
            backdrop-filter: blur(16px);
            -webkit-backdrop-filter: blur(16px);
            transition: border-color 0.3s ease;
        }

        .player-card:hover {
            border-color: var(--border-glass-glow);
        }

        .media-wrapper {
            position: relative;
            width: 100%;
            background: #000000;
            display: flex;
            align-items: center;
            justify-content: center;
        }

        video {
            width: 100%;
            max-height: 580px;
            outline: none;
            display: block;
            border-radius: 20px 20px 0 0;
            background: #000;
        }

        .audio-wrapper {
            padding: 40px 24px;
            display: flex;
            flex-direction: column;
            align-items: center;
            gap: 20px;
            background: radial-gradient(circle at center, rgba(139, 92, 246, 0.15) 0%, rgba(0,0,0,0.6) 80%);
        }

        .audio-icon {
            font-size: 56px;
            animation: pulse 2s infinite ease-in-out;
        }

        audio {
            width: 100%;
            max-width: 600px;
            outline: none;
            filter: invert(0.9) hue-rotate(180deg);
        }

        .doc-wrapper {
            padding: 60px 24px;
            display: flex;
            flex-direction: column;
            align-items: center;
            gap: 16px;
            text-align: center;
        }

        .doc-icon {
            font-size: 64px;
            color: var(--accent-cyan);
        }

        /* Info & Actions */
        .details-card {
            background: var(--bg-card);
            border: 1px solid var(--border-glass);
            border-radius: 20px;
            padding: 24px 28px;
            display: flex;
            flex-direction: column;
            gap: 20px;
            backdrop-filter: blur(16px);
            -webkit-backdrop-filter: blur(16px);
        }

        .file-header {
            display: flex;
            flex-direction: column;
            gap: 8px;
        }

        .file-title {
            font-size: 20px;
            font-weight: 700;
            line-height: 1.4;
            word-break: break-all;
            color: #ffffff;
        }

        .meta-badges {
            display: flex;
            flex-wrap: wrap;
            gap: 10px;
            align-items: center;
        }

        .badge {
            display: inline-flex;
            align-items: center;
            gap: 6px;
            padding: 4px 12px;
            border-radius: 8px;
            font-size: 13px;
            font-weight: 600;
            background: rgba(255, 255, 255, 0.06);
            border: 1px solid var(--border-glass);
            color: var(--text-muted);
        }

        .badge-size {
            color: #38bdf8;
            background: rgba(56, 189, 248, 0.1);
            border-color: rgba(56, 189, 248, 0.25);
        }

        .badge-hd {
            color: #34d399;
            background: rgba(52, 211, 153, 0.1);
            border-color: rgba(52, 211, 153, 0.25);
        }

        .badge-type {
            color: #c084fc;
            background: rgba(192, 132, 252, 0.1);
            border-color: rgba(192, 132, 252, 0.25);
        }

        /* Primary Action Buttons */
        .action-row {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
            gap: 14px;
        }

        .btn {
            display: inline-flex;
            align-items: center;
            justify-content: center;
            gap: 10px;
            padding: 14px 22px;
            border-radius: 12px;
            font-size: 15px;
            font-weight: 700;
            text-decoration: none;
            cursor: pointer;
            border: none;
            transition: all 0.25s cubic-bezier(0.16, 1, 0.3, 1);
            position: relative;
            overflow: hidden;
        }

        .btn-download {
            background: var(--primary-gradient);
            color: #ffffff;
            box-shadow: 0 8px 24px rgba(99, 102, 241, 0.35);
        }

        .btn-download:hover {
            transform: translateY(-2px);
            box-shadow: 0 12px 30px rgba(99, 102, 241, 0.5);
            filter: brightness(1.08);
        }

        .btn-secondary {
            background: rgba(255, 255, 255, 0.05);
            border: 1px solid var(--border-glass);
            color: var(--text-main);
        }

        .btn-secondary:hover {
            background: rgba(255, 255, 255, 0.1);
            border-color: rgba(255, 255, 255, 0.2);
            transform: translateY(-2px);
        }

        /* External Players Grid */
        .external-players-section {
            background: var(--bg-card);
            border: 1px solid var(--border-glass);
            border-radius: 20px;
            padding: 24px 28px;
            display: flex;
            flex-direction: column;
            gap: 16px;
        }

        .section-title {
            font-size: 16px;
            font-weight: 700;
            letter-spacing: -0.3px;
            color: var(--text-muted);
            text-transform: uppercase;
        }

        .players-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
            gap: 12px;
        }

        .player-btn {
            display: flex;
            align-items: center;
            gap: 10px;
            padding: 12px 16px;
            border-radius: 12px;
            background: rgba(255, 255, 255, 0.03);
            border: 1px solid var(--border-glass);
            color: var(--text-main);
            text-decoration: none;
            font-size: 14px;
            font-weight: 600;
            transition: all 0.2s ease;
        }

        .player-btn:hover {
            background: rgba(255, 255, 255, 0.08);
            border-color: var(--border-glass-glow);
            transform: translateY(-1px);
        }

        .player-btn-primary {
            background: rgba(249, 115, 22, 0.15) !important;
            border-color: rgba(249, 115, 22, 0.45) !important;
            color: #fb923c !important;
        }

        .player-btn-primary:hover {
            background: rgba(249, 115, 22, 0.28) !important;
            box-shadow: 0 4px 16px rgba(249, 115, 22, 0.35);
        }

        .format-banner {
            display: none;
            align-items: center;
            justify-content: space-between;
            gap: 16px;
            padding: 16px 20px;
            border-radius: 16px;
            background: linear-gradient(135deg, rgba(234, 88, 12, 0.16) 0%, rgba(245, 158, 11, 0.1) 100%);
            border: 1px solid rgba(245, 158, 11, 0.35);
        }

        .format-banner.show-banner {
            display: flex;
        }

        .format-banner.error-state {
            display: flex;
            background: linear-gradient(135deg, rgba(239, 68, 68, 0.18) 0%, rgba(220, 38, 38, 0.1) 100%);
            border-color: rgba(239, 68, 68, 0.45);
        }

        .banner-icon {
            font-size: 28px;
            flex-shrink: 0;
        }

        .banner-content {
            flex: 1;
            display: flex;
            flex-direction: column;
            gap: 4px;
        }

        .banner-title {
            font-size: 15px;
            font-weight: 700;
            color: #fbbf24;
        }

        .format-banner.error-state .banner-title {
            color: #f87171;
        }

        .banner-text {
            font-size: 13px;
            color: var(--text-muted);
            line-height: 1.4;
        }

        .banner-actions {
            display: flex;
            gap: 10px;
            flex-wrap: wrap;
            flex-shrink: 0;
        }

        .banner-btn {
            display: inline-flex;
            align-items: center;
            gap: 6px;
            padding: 8px 16px;
            border-radius: 10px;
            font-size: 13px;
            font-weight: 700;
            text-decoration: none;
            cursor: pointer;
            border: none;
            transition: all 0.2s ease;
        }

        .banner-btn.vlc-btn {
            background: #f97316;
            color: #ffffff;
            box-shadow: 0 4px 14px rgba(249, 115, 22, 0.4);
        }

        .banner-btn.vlc-btn:hover {
            background: #ea580c;
            transform: translateY(-1px);
        }

        .banner-btn.copy-btn {
            background: rgba(255, 255, 255, 0.08);
            color: var(--text-main);
            border: 1px solid var(--border-glass);
        }

        .banner-btn.copy-btn:hover {
            background: rgba(255, 255, 255, 0.18);
        }

        /* Direct link copy boxes */
        .url-box {
            display: flex;
            gap: 10px;
            align-items: center;
            background: rgba(0, 0, 0, 0.35);
            border: 1px solid var(--border-glass);
            border-radius: 12px;
            padding: 8px 12px;
        }

        .url-input {
            flex: 1;
            background: transparent;
            border: none;
            color: var(--text-muted);
            font-family: 'JetBrains Mono', monospace;
            font-size: 13px;
            outline: none;
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
        }

        .copy-icon-btn {
            background: rgba(255, 255, 255, 0.08);
            border: 1px solid var(--border-glass);
            border-radius: 8px;
            padding: 6px 12px;
            color: var(--text-main);
            font-size: 12px;
            font-weight: 600;
            cursor: pointer;
            transition: background 0.2s;
        }

        .copy-icon-btn:hover {
            background: rgba(255, 255, 255, 0.16);
        }

        /* Toast notification */
        .toast {
            position: fixed;
            bottom: 30px;
            left: 50%;
            transform: translateX(-50%) translateY(100px);
            background: #1e1b4b;
            border: 1px solid #6366f1;
            color: #ffffff;
            padding: 12px 24px;
            border-radius: 9999px;
            font-size: 14px;
            font-weight: 600;
            box-shadow: 0 10px 30px rgba(0, 0, 0, 0.5);
            opacity: 0;
            pointer-events: none;
            transition: all 0.35s cubic-bezier(0.16, 1, 0.3, 1);
            z-index: 1000;
            display: flex;
            align-items: center;
            gap: 8px;
        }

        .toast.show {
            transform: translateX(-50%) translateY(0);
            opacity: 1;
        }

        @keyframes pulse {
            0%, 100% { transform: scale(1); opacity: 0.8; }
            50% { transform: scale(1.08); opacity: 1; }
        }

        footer {
            margin-top: auto;
            padding: 24px;
            font-size: 13px;
            color: var(--text-subtle);
            text-align: center;
        }

        @media (max-width: 640px) {
            .file-title { font-size: 17px; }
            .action-row { grid-template-columns: 1fr; }
            .header { padding: 16px; }
            .details-card { padding: 18px; }
        }
    </style>
</head>
<body>

    <header class="header">
        <a href="/" class="brand">
            <div class="brand-badge">⚡</div>
            <div class="brand-text">High-Speed Stream</div>
        </a>
        {{ if .BotUsername }}
        <a href="https://t.me/{{ .BotUsername }}" target="_blank" class="tg-btn">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
                <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm4.64 6.8c-.15 1.58-.8 5.42-1.13 7.19-.14.75-.42 1-.68 1.03-.58.05-1.02-.38-1.58-.75-.88-.58-1.38-.94-2.23-1.5-.99-.65-.35-1.01.22-1.59.15-.15 2.71-2.48 2.76-2.69a.2.2 0 00-.05-.18c-.06-.05-.14-.03-.21-.02-.09.02-1.49.95-4.22 2.79-.4.27-.76.41-1.08.4-.36-.01-1.04-.2-1.55-.37-.63-.2-1.12-.31-1.08-.66.02-.18.27-.36.75-.55 2.92-1.27 4.86-2.11 5.83-2.51 2.78-1.16 3.35-1.36 3.73-1.36.08 0 .27.02.39.12.1.08.13.19.14.27-.01.06.01.24 0 .38z"/>
            </svg>
            Telegram Bot
        </a>
        {{ end }}
    </header>

    <main class="main-container">
        <!-- Media Player -->
        <section class="player-card">
            {{ if .IsVideo }}
            <div class="media-wrapper">
                <video id="videoPlayer" controls preload="metadata" playsinline src="{{ .StreamURL }}">
                    <source src="{{ .StreamURL }}" {{ if not .IsMKV }}type="{{ .MimeType }}"{{ end }}>
                    Your browser does not support HTML5 video streaming. Please use the Fast Download or VLC buttons below.
                </video>
            </div>
            <div class="format-banner {{ if .IsMKV }}show-banner{{ end }}" id="formatBanner" style="margin: 16px 20px 20px;">
                <div class="banner-icon">💡</div>
                <div class="banner-content">
                    <div class="banner-title" id="bannerTitle">
                        {{ if .IsMKV }}MKV / 10-Bit Container Detected{{ else }}External Player Recommended{{ end }}
                    </div>
                    <p class="banner-text" id="bannerText">
                        Web browsers lack native decoders for 10-bit MKV video & multi-channel AAC audio.
                        Click <b>Stream in VLC</b> for instant 1080p playback, or use <b>Fast Direct Download</b>.
                    </p>
                </div>
                <div class="banner-actions">
                    <a href="{{ .VLCUrl }}" class="banner-btn vlc-btn">
                        <span>🟧</span> Open in VLC
                    </a>
                    <button class="banner-btn copy-btn" onclick="copyVLCCommand('{{ .StreamURL }}')">
                        <span>📋</span> Copy Stream Link
                    </button>
                </div>
            </div>
            {{ else if .IsAudio }}
            <div class="audio-wrapper">
                <div class="audio-icon">🎵</div>
                <audio id="audioPlayer" controls preload="metadata">
                    <source src="{{ .StreamURL }}" type="{{ .MimeType }}">
                    Your browser does not support HTML5 audio playback.
                </audio>
            </div>
            {{ else }}
            <div class="doc-wrapper">
                <div class="doc-icon">📁</div>
                <h3>{{ .FileName }}</h3>
                <p style="color: var(--text-muted); font-size: 14px;">Preview not supported in browser for this file type. Click Fast Download to get it.</p>
            </div>
            {{ end }}
        </section>

        <!-- File Details & Main Actions -->
        <section class="details-card">
            <div class="file-header">
                <h1 class="file-title">{{ .FileName }}</h1>
                <div class="meta-badges">
                    <span class="badge badge-size">📦 {{ .FileSizeStr }}</span>
                    <span class="badge badge-type">🏷️ {{ .FileType }}</span>
                    {{ if .IsVideo }}
                    <span class="badge badge-hd">🎬 Full HD Source Quality</span>
                    {{ end }}
                    <span class="badge">⚡ VPS Ultra-Fast Bandwidth</span>
                </div>
            </div>

            <!-- Action Buttons -->
            <div class="action-row">
                <a href="{{ .DownloadURL }}" class="btn btn-download">
                    <span>🚀 Fast Direct Download</span>
                </a>
                <button class="btn btn-secondary" onclick="copyToClipboard('{{ .StreamURL }}', 'Stream link copied to clipboard!')">
                    <span>📋 Copy Stream Link</span>
                </button>
                <button class="btn btn-secondary" onclick="copyToClipboard('{{ .DownloadURL }}', 'Download link copied to clipboard!')">
                    <span>📋 Copy Download Link</span>
                </button>
            </div>

            <!-- Direct URL Display -->
            <div style="display: flex; flex-direction: column; gap: 8px;">
                <label style="font-size: 12px; font-weight: 600; color: var(--text-subtle); text-transform: uppercase;">Direct Streaming URL</label>
                <div class="url-box">
                    <input type="text" class="url-input" readonly value="{{ .StreamURL }}" id="streamInput" onclick="this.select()">
                    <button class="copy-icon-btn" onclick="copyFromInput('streamInput')">Copy</button>
                </div>
            </div>
        </section>

        <!-- External Players Integration -->
        {{ if or .IsVideo .IsAudio }}
        <section class="external-players-section">
            <div class="section-title">Play in External App</div>
            <div class="players-grid">
                <a href="{{ .VLCUrl }}" class="player-btn player-btn-primary">
                    <span>🟧</span> Open in VLC
                </a>
                <button class="player-btn" onclick="copyVLCCommand('{{ .StreamURL }}')">
                    <span>📋</span> Copy Stream URL (Ctrl+N)
                </button>
                <a href="{{ .MXPlayerUrl }}" class="player-btn">
                    <span>▶️</span> MX Player (Android)
                </a>
                <a href="{{ .PlayItUrl }}" class="player-btn">
                    <span>⚡</span> PlayIt Player
                </a>
                {{ if .BotUsername }}
                <a href="https://t.me/{{ .BotUsername }}" target="_blank" class="player-btn">
                    <span>🤖</span> Telegram Bot
                </a>
                {{ end }}
            </div>
        </section>
        {{ end }}
    </main>

    <div class="toast" id="toast">
        <span>✓</span> <span id="toastMsg">Copied!</span>
    </div>

    <footer>
        High-Performance Fast Streaming & Download Service Powered by Telegram MTProto
    </footer>

    <script>
        function showToast(msg) {
            const toast = document.getElementById('toast');
            const toastMsg = document.getElementById('toastMsg');
            toastMsg.innerText = msg;
            toast.classList.add('show');
            setTimeout(() => {
                toast.classList.remove('show');
            }, 2500);
        }

        function copyToClipboard(text, msg) {
            if (navigator.clipboard && navigator.clipboard.writeText) {
                navigator.clipboard.writeText(text).then(() => {
                    showToast(msg);
                }).catch(() => {
                    fallbackCopy(text, msg);
                });
            } else {
                fallbackCopy(text, msg);
            }
        }

        function copyFromInput(id) {
            const input = document.getElementById(id);
            if (input) {
                input.select();
                document.execCommand('copy');
                showToast('Link copied to clipboard!');
            }
        }

        function fallbackCopy(text, msg) {
            const el = document.createElement('textarea');
            el.value = text;
            document.body.appendChild(el);
            el.select();
            document.execCommand('copy');
            document.body.removeChild(el);
            showToast(msg);
        }

        function copyVLCCommand(url) {
            copyToClipboard(url, 'Stream link copied! Open VLC -> Press Ctrl+N -> Paste & Stream!');
        }

        // Video Player handling and Error Detection
        const video = document.getElementById('videoPlayer');
        const banner = document.getElementById('formatBanner');

        if (video) {
            video.addEventListener('error', function() {
                if (banner) {
                    banner.classList.add('error-state', 'show-banner');
                    const title = document.getElementById('bannerTitle');
                    if (title) title.innerText = 'Browser Cannot Decode This Video Format';
                    const text = document.getElementById('bannerText');
                    if (text) text.innerText = 'This browser does not support decoding this video container or codec. Click Stream in VLC or Fast Direct Download!';
                }
            });
            document.addEventListener('keydown', (e) => {
                if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA') return;
                switch (e.key) {
                    case ' ':
                        e.preventDefault();
                        if (video.paused) video.play(); else video.pause();
                        break;
                    case 'ArrowLeft':
                        e.preventDefault();
                        video.currentTime = Math.max(0, video.currentTime - 10);
                        break;
                    case 'ArrowRight':
                        e.preventDefault();
                        video.currentTime = Math.min(video.duration, video.currentTime + 10);
                        break;
                    case 'f':
                    case 'F':
                        e.preventDefault();
                        if (document.fullscreenElement) {
                            document.exitFullscreen();
                        } else {
                            video.requestFullscreen().catch(() => {});
                        }
                        break;
                    case 'm':
                    case 'M':
                        e.preventDefault();
                        video.muted = !video.muted;
                        break;
                }
            });
        }
    </script>
</body>
</html>
`

var parsedWatchTemplate = template.Must(template.New("watch").Parse(watchTemplateHTML))

// RenderWatchPage renders the HTML watch and fast download page.
func RenderWatchPage(w io.Writer, data PageData) error {
	return parsedWatchTemplate.Execute(w, data)
}
