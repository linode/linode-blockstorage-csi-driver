package driver

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/linode/linodego/v2"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/linode/linode-blockstorage-csi-driver/mocks"
)

// The CSI volume ID "1003" (without a label) is parsed to this Linode volume ID.
const unpublishTestVolumeID = 630706045

// withFastDetachRetries shortens the detach retry interval and timeout for a test.
func withFastDetachRetries(t *testing.T, interval, timeout time.Duration) {
	t.Helper()
	oldInterval, oldTimeout := detachRetryInterval, detachTimeout
	detachRetryInterval, detachTimeout = interval, timeout
	t.Cleanup(func() { detachRetryInterval, detachTimeout = oldInterval, oldTimeout })
}

// waitUntilDeadline simulates a volume that never detaches: the wait only
// returns once its context expires.
func waitUntilDeadline(ctx context.Context, _ int, _ *int) (*linodego.Volume, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func attachedVolume(linodeID int) *linodego.Volume {
	return &linodego.Volume{ID: unpublishTestVolumeID, LinodeID: createLinodeID(linodeID), Size: 10, Status: linodego.VolumeActive}
}

func TestControllerUnpublishVolumeDetachRetries(t *testing.T) {
	tests := []struct {
		name         string
		expectCalls  func(m *mocks.MockLinodeClient)
		wantCode     codes.Code
		wantErrMatch string
	}{
		{
			name: "detach completes on the first attempt",
			expectCalls: func(m *mocks.MockLinodeClient) {
				gomock.InOrder(
					m.EXPECT().GetVolume(gomock.Any(), unpublishTestVolumeID).Return(attachedVolume(1003), nil),
					m.EXPECT().DetachVolume(gomock.Any(), unpublishTestVolumeID).Return(nil),
					m.EXPECT().WaitForVolumeLinodeID(gomock.Any(), unpublishTestVolumeID, nil).Return(&linodego.Volume{ID: unpublishTestVolumeID}, nil),
				)
			},
			wantCode: codes.OK,
		},
		{
			name: "stuck detach is re-issued and then completes",
			expectCalls: func(m *mocks.MockLinodeClient) {
				gomock.InOrder(
					m.EXPECT().GetVolume(gomock.Any(), unpublishTestVolumeID).Return(attachedVolume(1003), nil),
					m.EXPECT().DetachVolume(gomock.Any(), unpublishTestVolumeID).Return(nil),
					m.EXPECT().WaitForVolumeLinodeID(gomock.Any(), unpublishTestVolumeID, nil).DoAndReturn(waitUntilDeadline),
					m.EXPECT().GetVolume(gomock.Any(), unpublishTestVolumeID).Return(attachedVolume(1003), nil),
					m.EXPECT().DetachVolume(gomock.Any(), unpublishTestVolumeID).Return(nil),
					m.EXPECT().WaitForVolumeLinodeID(gomock.Any(), unpublishTestVolumeID, nil).Return(&linodego.Volume{ID: unpublishTestVolumeID}, nil),
				)
			},
			wantCode: codes.OK,
		},
		{
			name: "error from a re-issued detach is tolerated",
			expectCalls: func(m *mocks.MockLinodeClient) {
				gomock.InOrder(
					m.EXPECT().GetVolume(gomock.Any(), unpublishTestVolumeID).Return(attachedVolume(1003), nil),
					m.EXPECT().DetachVolume(gomock.Any(), unpublishTestVolumeID).Return(nil),
					m.EXPECT().WaitForVolumeLinodeID(gomock.Any(), unpublishTestVolumeID, nil).DoAndReturn(waitUntilDeadline),
					m.EXPECT().GetVolume(gomock.Any(), unpublishTestVolumeID).Return(attachedVolume(1003), nil),
					m.EXPECT().DetachVolume(gomock.Any(), unpublishTestVolumeID).Return(&linodego.Error{Code: 400, Message: "volume is busy"}),
					m.EXPECT().WaitForVolumeLinodeID(gomock.Any(), unpublishTestVolumeID, nil).Return(&linodego.Volume{ID: unpublishTestVolumeID}, nil),
				)
			},
			wantCode: codes.OK,
		},
		{
			name: "volume detached between attempts is not detached again",
			expectCalls: func(m *mocks.MockLinodeClient) {
				gomock.InOrder(
					m.EXPECT().GetVolume(gomock.Any(), unpublishTestVolumeID).Return(attachedVolume(1003), nil),
					m.EXPECT().DetachVolume(gomock.Any(), unpublishTestVolumeID).Return(nil),
					m.EXPECT().WaitForVolumeLinodeID(gomock.Any(), unpublishTestVolumeID, nil).DoAndReturn(waitUntilDeadline),
					m.EXPECT().GetVolume(gomock.Any(), unpublishTestVolumeID).Return(&linodego.Volume{ID: unpublishTestVolumeID}, nil),
				)
			},
			wantCode: codes.OK,
		},
		{
			name: "volume deleted between attempts is not detached again",
			expectCalls: func(m *mocks.MockLinodeClient) {
				gomock.InOrder(
					m.EXPECT().GetVolume(gomock.Any(), unpublishTestVolumeID).Return(attachedVolume(1003), nil),
					m.EXPECT().DetachVolume(gomock.Any(), unpublishTestVolumeID).Return(nil),
					m.EXPECT().WaitForVolumeLinodeID(gomock.Any(), unpublishTestVolumeID, nil).DoAndReturn(waitUntilDeadline),
					m.EXPECT().GetVolume(gomock.Any(), unpublishTestVolumeID).Return(nil, &linodego.Error{Code: 404}),
				)
			},
			wantCode: codes.OK,
		},
		{
			name: "volume attached to another instance meanwhile is never detached from it",
			expectCalls: func(m *mocks.MockLinodeClient) {
				gomock.InOrder(
					m.EXPECT().GetVolume(gomock.Any(), unpublishTestVolumeID).Return(attachedVolume(1003), nil),
					m.EXPECT().DetachVolume(gomock.Any(), unpublishTestVolumeID).Return(nil),
					m.EXPECT().WaitForVolumeLinodeID(gomock.Any(), unpublishTestVolumeID, nil).DoAndReturn(waitUntilDeadline),
					m.EXPECT().GetVolume(gomock.Any(), unpublishTestVolumeID).Return(attachedVolume(2002), nil),
				)
			},
			wantCode: codes.OK,
		},
		{
			name: "first detach request failing is returned immediately",
			expectCalls: func(m *mocks.MockLinodeClient) {
				gomock.InOrder(
					m.EXPECT().GetVolume(gomock.Any(), unpublishTestVolumeID).Return(attachedVolume(1003), nil),
					m.EXPECT().DetachVolume(gomock.Any(), unpublishTestVolumeID).Return(errors.New("boom")),
				)
			},
			wantCode:     codes.Internal,
			wantErrMatch: "detach volume 630706045: boom",
		},
		{
			name: "volume that never detaches fails after the timeout",
			expectCalls: func(m *mocks.MockLinodeClient) {
				m.EXPECT().GetVolume(gomock.Any(), unpublishTestVolumeID).Return(attachedVolume(1003), nil).MinTimes(2)
				m.EXPECT().DetachVolume(gomock.Any(), unpublishTestVolumeID).Return(nil).MinTimes(2)
				m.EXPECT().WaitForVolumeLinodeID(gomock.Any(), unpublishTestVolumeID, nil).DoAndReturn(waitUntilDeadline).MinTimes(2)
			},
			wantCode:     codes.Internal,
			wantErrMatch: "volume still attached after",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFastDetachRetries(t, 20*time.Millisecond, 200*time.Millisecond)

			ctrl := gomock.NewController(t)
			mockClient := mocks.NewMockLinodeClient(ctrl)
			tt.expectCalls(mockClient)

			s := &ControllerServer{client: mockClient, driver: &LinodeDriver{}}
			_, err := s.ControllerUnpublishVolume(context.Background(), &csi.ControllerUnpublishVolumeRequest{
				VolumeId: "1003",
				NodeId:   "1003",
			})

			if got := status.Code(err); got != tt.wantCode {
				t.Fatalf("status code = %v, want %v (err: %v)", got, tt.wantCode, err)
			}
			if tt.wantErrMatch != "" && !strings.Contains(err.Error(), tt.wantErrMatch) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErrMatch)
			}
		})
	}
}
