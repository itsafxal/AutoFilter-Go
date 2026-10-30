package batch

import (
	"fmt"

	"github.com/itsafxal/AutoFilter-Go/internal/commands"
)

// FormatDeliveryStartedText constructs the initial temporary status message text
// sent when a user requests delivery of a batch collection.
func FormatDeliveryStartedText(batchName string, totalItems int) string {
	itemUnit := "items"
	if totalItems == 1 {
		itemUnit = "item"
	}
	return fmt.Sprintf(
		"📦 <b>Batch Delivery Started!</b>\n\n"+
			"<b>Batch:</b> %s\n"+
			"⏳ Preparing to deliver %d %s...",
		commands.EscapeHTML(batchName),
		totalItems,
		itemUnit,
	)
}

// FormatDeliveryCompleteText constructs the final completion message text
// sent below all delivered files after delivery finishes.
func FormatDeliveryCompleteText(batchName string, deliveredCount, totalItems int) string {
	failedCount := totalItems - deliveredCount
	if failedCount < 0 {
		failedCount = 0
	}

	if failedCount == 0 {
		return fmt.Sprintf(
			"✨ <b>Batch Delivery Complete!</b>\n\n"+
				"📦 <b>Batch:</b> %s\n"+
				"✅ <b>Delivered:</b> %d / %d items",
			commands.EscapeHTML(batchName),
			deliveredCount,
			totalItems,
		)
	}

	return fmt.Sprintf(
		"⚠️ <b>Batch Delivery Completed with Errors</b>\n\n"+
			"📦 <b>Batch:</b> %s\n"+
			"✅ <b>Delivered:</b> %d\n"+
			"❌ <b>Failed:</b> %d\n"+
			"📁 <b>Total:</b> %d items",
		commands.EscapeHTML(batchName),
		deliveredCount,
		failedCount,
		totalItems,
	)
}
