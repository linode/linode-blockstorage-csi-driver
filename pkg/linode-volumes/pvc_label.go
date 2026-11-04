package linodevolumes

import (
	"regexp"
	"strings"
)

const (
	// PVCNameKey and PVCNamespaceKey are the CreateVolume parameters the
	// external-provisioner adds when it runs with --extra-create-metadata.
	PVCNameKey      = "csi.storage.k8s.io/pvc/name"
	PVCNamespaceKey = "csi.storage.k8s.io/pvc/namespace"

	// pvcLabelSuffixLength is how many trailing characters of the CSI volume
	// name (pvc-<uid>) are kept in a PVC-derived label. They keep labels
	// unique when two PVCs share a name (for example a PVC that was deleted
	// and recreated while its old Retain volume still exists) and when long
	// names have to be shortened.
	pvcLabelSuffixLength = 8
)

// volumeLabelRegexp is the label pattern enforced by the Linode API for volumes.
var volumeLabelRegexp = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]+$`)

// PVCVolumeLabel builds a Linode volume label of the form
// <prefix><namespace>-<pvc name>-<suffix>, where suffix is the tail of the
// CSI volume name (pvc-<uid>). When the label would be longer than
// LinodeVolumeLabelLength, characters are dropped from the middle of
// "<namespace>-<pvc name>" so that the start (namespace) and the end
// (for example a StatefulSet ordinal) both survive.
//
// It returns false when a valid label cannot be built, in which case the
// caller should fall back to the default label.
func PVCVolumeLabel(prefix, namespace, pvcName, volumeName string) (string, bool) {
	if namespace == "" || pvcName == "" {
		return "", false
	}

	suffix := alphanumericTail(volumeName, pvcLabelSuffixLength)
	if suffix == "" {
		return "", false
	}

	ident := sanitizeLabelPart(namespace + "-" + pvcName)
	budget := LinodeVolumeLabelLength - len(prefix) - len(suffix) - 1
	if ident == "" || budget < 1 {
		return "", false
	}
	if len(ident) > budget {
		ident = truncateMiddle(ident, budget)
	}

	label := collapseSeparators(prefix + ident + "-" + suffix)
	if len(label) > LinodeVolumeLabelLength || !isValidVolumeLabel(label) {
		return "", false
	}
	return label, true
}

// truncateMiddle shortens ident to at most n characters by removing characters
// from the middle and joining the two ends with a dash.
func truncateMiddle(ident string, n int) string {
	if n < 3 {
		return trimSeparators(ident[len(ident)-n:])
	}
	head := (n - 1) / 2
	tail := n - 1 - head
	out := trimSeparators(ident[:head]) + "-" + trimSeparators(ident[len(ident)-tail:])
	return trimSeparators(collapseSeparators(out))
}

// sanitizeLabelPart maps characters the Linode API does not accept in a
// label (Kubernetes names may contain dots) to dashes, collapses repeated
// separators and trims separators from both ends.
func sanitizeLabelPart(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			out.WriteRune(r)
		default:
			out.WriteByte('-')
		}
	}
	return trimSeparators(collapseSeparators(out.String()))
}

// collapseSeparators replaces runs of "-" or "_" with a single character,
// since the Linode API rejects labels containing "--" or "__".
func collapseSeparators(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	var prev byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c == '-' || c == '_') && c == prev {
			continue
		}
		out.WriteByte(c)
		prev = c
	}
	return out.String()
}

func trimSeparators(s string) string {
	return strings.Trim(s, "-_")
}

// alphanumericTail returns the last n letters or digits of s.
func alphanumericTail(s string, n int) string {
	out := make([]byte, 0, n)
	for i := len(s) - 1; i >= 0 && len(out) < n; i-- {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			out = append(out, c)
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return string(out)
}

func isValidVolumeLabel(label string) bool {
	return volumeLabelRegexp.MatchString(label) &&
		!strings.Contains(label, "--") &&
		!strings.Contains(label, "__")
}
