// applyMiddleware runs guardrail redaction and context compression on the request.
func (h *Handler) applyMiddleware(req *models.ChatCompletionRequest) (*models.ChatCompletionRequest, *guardrail.RedactionMap) {
	out := req.Clone()
	combinedMap := guardrail.NewRedactionMap()

	for i, msg := range out.Messages {
		redacted, rm := guardrail.Redact(msg.Content)
		out.Messages[i].Content = redacted
		combinedMap.Merge(rm)
	}

	for i, msg := range out.Messages {
		if msg.Role == "system" || msg.Role == "user" {
			if pruned, ok := h.compressor.CompressMessage(msg.Content); ok {
				out.Messages[i].Content = pruned
			}
		}
	}

	return out, combinedMap
}
