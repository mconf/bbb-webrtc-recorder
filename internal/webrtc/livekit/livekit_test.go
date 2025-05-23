package livekit

import (
	"context"
	"testing"
	"time"

	"github.com/bigbluebutton/bbb-webrtc-recorder/internal/appstats"
	"github.com/bigbluebutton/bbb-webrtc-recorder/internal/config"
	"github.com/bigbluebutton/bbb-webrtc-recorder/internal/webrtc/interfaces"
	"github.com/bigbluebutton/bbb-webrtc-recorder/internal/webrtc/recorder"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

// mockRecorder implements the recorder.Recorder interface for testing
type mockRecorder struct {
	videoStats *recorder.RecorderTrackStats
	audioStats *recorder.RecorderTrackStats
	hasAudio   bool
	hasVideo   bool
	filePath   string
}

func (m *mockRecorder) GetFilePath() string {
	return m.filePath
}

func (m *mockRecorder) GetStats() *recorder.RecorderStats {
	return &recorder.RecorderStats{
		Video: m.videoStats,
		Audio: m.audioStats,
	}
}

func (m *mockRecorder) PushVideo(packet *rtp.Packet)                                {}
func (m *mockRecorder) PushAudio(packet *rtp.Packet)                                {}
func (m *mockRecorder) NotifySkippedPacket(seq uint16)                              {}
func (m *mockRecorder) WithContext(ctx context.Context)                             {}
func (m *mockRecorder) VideoTimestamp() time.Duration                               { return 0 }
func (m *mockRecorder) AudioTimestamp() time.Duration                               { return 0 }
func (m *mockRecorder) SetHasAudio(hasAudio bool)                                   { m.hasAudio = hasAudio }
func (m *mockRecorder) SetHasVideo(hasVideo bool)                                   { m.hasVideo = hasVideo }
func (m *mockRecorder) SetKeyframeRequester(requester interfaces.KeyframeRequester) {}
func (m *mockRecorder) GetHasAudio() bool                                           { return m.hasAudio }
func (m *mockRecorder) GetHasVideo() bool                                           { return m.hasVideo }
func (m *mockRecorder) Close() time.Duration                                        { return 0 }

func TestProcessPacketStats_SequenceNumberWraparound(t *testing.T) {
	lk, _ := setupMockLK()
	trackIds := lk.trackIds
	packets := makeFullRangePackets(0)

	// Verify wraparound count 0 - we should be at 65530
	lk.processPacketStats(trackIds[0], packets[:65531])
	trackStats := lk.trackStats[trackIds[0]]
	assert.Equal(t, 0, trackStats.SeqNumWrapArounds,
		"Should detect no sequence number wraparounds")

	// Wrap around exactly to zero
	lk.processPacketStats(trackIds[0], append(packets[65530:], packets[:1]...))
	assert.Equal(t, 1, trackStats.SeqNumWrapArounds,
		"Should detect one sequence number wraparound (exactly to zero)")

	// Induce two more wraparounds by processsing the same packets again three times.
	// Do it in batches of 250 packets to test the wraparound detection logic.
	// We should end at 65534
	for i := 0; i < 3; i++ {
		clonedPackets := make([]*rtp.Packet, len(packets))
		copy(clonedPackets, packets)
		for j := 0; j < len(clonedPackets); j += 250 {
			end := j + 250

			if end > len(clonedPackets) {
				end = len(clonedPackets)
			}
			lk.processPacketStats(trackIds[0], clonedPackets[j:end])
		}
	}

	assert.Equal(t, 3, trackStats.SeqNumWrapArounds,
		"Should detect 3 sequence number wraparounds")
}

// Test wraparound detection with pre-initialized track stats
func TestProcessPacketStats_PreInitializedTrackStatsWraparound(t *testing.T) {
	lk, _ := setupMockLK()
	trackIds := lk.trackIds
	packets := makeFullRangePackets(0)
	lk.trackStats[trackIds[0]] = &appstats.AdapterTrackStats{
		StartTime:         time.Now().Unix(),
		EndTime:           time.Now().Unix(),
		FirstSeqNum:       0,
		LastSeqNum:        0,
		SeqNumWrapArounds: 0,
		PLIRequests:       0,
		RTPReadErrors:     0,
	}
	// Process all initial packets (no wraparounds)
	lk.processPacketStats(trackIds[0], packets)
	assert.Equal(t, 0, lk.trackStats[trackIds[0]].SeqNumWrapArounds,
		"Should detect no sequence number wraparound")

	// Process packets with wraparound - from 65530 to 0 to 65531
	lk.processPacketStats(trackIds[0], append(packets[65530:], packets[:65532]...))
	assert.Equal(t, 1, lk.trackStats[trackIds[0]].SeqNumWrapArounds,
		"Should detect one sequence number wraparound")
}

func TestDoubleClose(t *testing.T) {
	lk, _ := setupMockLK()

	// First close should succeed
	firstDuration := lk.Close()
	assert.Equal(t, time.Duration(0), firstDuration, "First close should return duration from recorder")

	time.Sleep(50 * time.Millisecond)

	// Second close should not panic
	secondDuration := lk.Close()
	assert.Equal(t, time.Duration(0), secondDuration, "Second close should still return same duration")
	assert.Nil(t, lk.room, "Room should be nil after close")
	assert.NotNil(t, lk.keyframeRequestChan, "Channel should not be nil even after close")
}

func setupMockLK() (*LiveKitWebRTC, *mockRecorder) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, "session", "test-session")
	cfg := config.LiveKit{
		Host:      "test-host",
		APIKey:    "test-key",
		APISecret: "test-secret",
	}
	rec := &mockRecorder{
		videoStats: &recorder.RecorderTrackStats{},
		audioStats: &recorder.RecorderTrackStats{},
		filePath:   "test.webm",
	}
	roomID := "test-room"
	trackIDs := []string{"test-track"}
	lk := NewLiveKitWebRTC(ctx, cfg, rec, roomID, trackIDs)

	return lk, rec
}

func makeFullRangePackets(start uint16) []*rtp.Packet {
	packets := make([]*rtp.Packet, 65535)

	for i := 0; i < 65535; i++ {
		packets[i] = &rtp.Packet{
			Header: rtp.Header{
				SequenceNumber: uint16(i),
				Timestamp:      uint32(i * 40),
			},
		}
	}

	return packets
}
