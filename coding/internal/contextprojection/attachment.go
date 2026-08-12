package contextprojection

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

type trackedAttachment struct {
	Attachment
	committed bool
}

func newTracked(
	epoch uint64,
	kind AttachmentKind,
	placement Placement,
	revision string,
	rendered string,
) *trackedAttachment {
	return &trackedAttachment{Attachment: Attachment{
		ID:        fmt.Sprintf("%s:%d:%s", kind, epoch, revision),
		Epoch:     epoch,
		Kind:      kind,
		Placement: placement,
		Revision:  revision,
		Rendered:  rendered,
	}}
}

func revisionOf(rendered string) string {
	sum := sha256.Sum256([]byte(rendered))
	return hex.EncodeToString(sum[:])
}
