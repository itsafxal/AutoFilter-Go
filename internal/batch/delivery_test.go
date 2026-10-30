package batch_test

import (
	"strings"
	"testing"

	"github.com/itsafxal/AutoFilter-Go/internal/batch"
)

func TestFormatDeliveryStartedText(t *testing.T) {
	// 1. Multi-item batch
	text := batch.FormatDeliveryStartedText("Somante Krithavu", 7)
	if !strings.Contains(text, "📦 <b>Batch Delivery Started!</b>") {
		t.Errorf("expected header '📦 <b>Batch Delivery Started!</b>', got:\n%s", text)
	}
	if !strings.Contains(text, "<b>Batch:</b> Somante Krithavu") {
		t.Errorf("expected batch name in text, got:\n%s", text)
	}
	if !strings.Contains(text, "⏳ Preparing to deliver 7 items...") {
		t.Errorf("expected '⏳ Preparing to deliver 7 items...', got:\n%s", text)
	}

	// 2. Single item batch
	textSingle := batch.FormatDeliveryStartedText("Interstellar", 1)
	if !strings.Contains(textSingle, "⏳ Preparing to deliver 1 item...") {
		t.Errorf("expected '⏳ Preparing to deliver 1 item...', got:\n%s", textSingle)
	}

	// 3. HTML escaping of malicious or dangerous batch names
	textEscaped := batch.FormatDeliveryStartedText("<script>alert('hack')</script> & \"Quotes\"", 5)
	if strings.Contains(textEscaped, "<script>") {
		t.Errorf("expected raw HTML tag to be escaped, got:\n%s", textEscaped)
	}
	if !strings.Contains(textEscaped, "&lt;script&gt;") {
		t.Errorf("expected HTML entity &lt;script&gt;, got:\n%s", textEscaped)
	}
	if !strings.Contains(textEscaped, "&amp;") {
		t.Errorf("expected HTML entity &amp;, got:\n%s", textEscaped)
	}
}

func TestFormatDeliveryCompleteText_Success(t *testing.T) {
	text := batch.FormatDeliveryCompleteText("Somante Krithavu", 7, 7)
	if !strings.Contains(text, "✨ <b>Batch Delivery Complete!</b>") {
		t.Errorf("expected success header '✨ <b>Batch Delivery Complete!</b>', got:\n%s", text)
	}
	if !strings.Contains(text, "📦 <b>Batch:</b> Somante Krithavu") {
		t.Errorf("expected batch name in text, got:\n%s", text)
	}
	if !strings.Contains(text, "✅ <b>Delivered:</b> 7 / 7 items") {
		t.Errorf("expected '✅ <b>Delivered:</b> 7 / 7 items', got:\n%s", text)
	}
	// Verify no error markers on full success
	if strings.Contains(text, "⚠️") || strings.Contains(text, "Failed") {
		t.Errorf("success text must not contain error markers, got:\n%s", text)
	}
}

func TestFormatDeliveryCompleteText_PartialFailure(t *testing.T) {
	text := batch.FormatDeliveryCompleteText("Somante Krithavu", 5, 7)
	if !strings.Contains(text, "⚠️ <b>Batch Delivery Completed with Errors</b>") {
		t.Errorf("expected warning header '⚠️ <b>Batch Delivery Completed with Errors</b>', got:\n%s", text)
	}
	if !strings.Contains(text, "📦 <b>Batch:</b> Somante Krithavu") {
		t.Errorf("expected batch name in text, got:\n%s", text)
	}
	if !strings.Contains(text, "✅ <b>Delivered:</b> 5") {
		t.Errorf("expected '✅ <b>Delivered:</b> 5', got:\n%s", text)
	}
	if !strings.Contains(text, "❌ <b>Failed:</b> 2") {
		t.Errorf("expected '❌ <b>Failed:</b> 2', got:\n%s", text)
	}
	if !strings.Contains(text, "📁 <b>Total:</b> 7 items") {
		t.Errorf("expected '📁 <b>Total:</b> 7 items', got:\n%s", text)
	}
}

func TestFormatDeliveryCompleteText_TotalFailure(t *testing.T) {
	text := batch.FormatDeliveryCompleteText("Somante Krithavu", 0, 7)
	if !strings.Contains(text, "⚠️ <b>Batch Delivery Completed with Errors</b>") {
		t.Errorf("expected warning header, got:\n%s", text)
	}
	if !strings.Contains(text, "✅ <b>Delivered:</b> 0") {
		t.Errorf("expected '✅ <b>Delivered:</b> 0', got:\n%s", text)
	}
	if !strings.Contains(text, "❌ <b>Failed:</b> 7") {
		t.Errorf("expected '❌ <b>Failed:</b> 7', got:\n%s", text)
	}
	if !strings.Contains(text, "📁 <b>Total:</b> 7 items") {
		t.Errorf("expected '📁 <b>Total:</b> 7 items', got:\n%s", text)
	}
}

func TestFormatDeliveryCompleteText_EdgeCases(t *testing.T) {
	// Single item success
	singleSuccess := batch.FormatDeliveryCompleteText("Single Movie", 1, 1)
	if !strings.Contains(singleSuccess, "✅ <b>Delivered:</b> 1 / 1 items") {
		t.Errorf("expected '1 / 1 items', got:\n%s", singleSuccess)
	}

	// Single item failure
	singleFail := batch.FormatDeliveryCompleteText("Single Movie", 0, 1)
	if !strings.Contains(singleFail, "❌ <b>Failed:</b> 1") || !strings.Contains(singleFail, "📁 <b>Total:</b> 1 items") {
		t.Errorf("expected failed single item output, got:\n%s", singleFail)
	}

	// Delivered count exceeding total (defensive check)
	overDelivered := batch.FormatDeliveryCompleteText("Odd Case", 10, 5)
	if !strings.Contains(overDelivered, "✨ <b>Batch Delivery Complete!</b>") {
		t.Errorf("expected success header for over-delivered case, got:\n%s", overDelivered)
	}
}
