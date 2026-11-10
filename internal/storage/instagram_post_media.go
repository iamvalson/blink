package storage

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/iamvalson/blink/internal/model"
)

// getInstagramPreviewMedia returns the exact assets sent to Instagram.
// A missing or incomplete upload keeps its slot so later items retain their order.
func (r *PostRepository) getInstagramPreviewMedia(ctx context.Context, userID, postID uuid.UUID) ([]model.PostAttachment, error) {
	rows, err := r.db.Query(ctx, `SELECT ipm.position,m.id,m.content_type
 FROM instagram_post_media ipm
 JOIN posts p ON p.id=ipm.post_id AND p.user_id=$2
 LEFT JOIN media m ON m.id=ipm.media_id AND m.user_id=p.user_id AND m.upload_status='complete'
 WHERE ipm.post_id=$1
 ORDER BY ipm.position`, postID, userID)
	if err != nil {
		return nil, fmt.Errorf("get Instagram post attachments: %w", err)
	}
	defer rows.Close()
	var attachments []model.PostAttachment
	for rows.Next() {
		var item model.PostAttachment
		if err := rows.Scan(&item.Position, &item.MediaID, &item.ContentType); err != nil {
			return nil, fmt.Errorf("scan Instagram attachment: %w", err)
		}
		attachments = append(attachments, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read Instagram post attachments: %w", err)
	}
	return attachments, nil
}
