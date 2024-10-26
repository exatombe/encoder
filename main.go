package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/schollz/progressbar/v3"
)

type Stream struct {
	Index     int    `json:"index"`
	CodecType string `json:"codec_type"`
	Tags      struct {
		Language string `json:"language"`
		Duration string `json:"DURATION"`
	} `json:"tags"`
}

type ProbeData struct {
	Streams []Stream `json:"streams"`
}

func main() {
	// Flags
	path := flag.String("path", "", "Path to the input video file")
	output := flag.String("output", "output.mp4", "Output filename")

	flag.Parse()

	if *path == "" {
		log.Fatal("Error: -path argument is required")
	}

	log.Printf("Starting video processing. Path: %s, Output: %s", *path, *output)

	// Step 1: Get stream list via ffprobe
	log.Println("Getting stream list via ffprobe")
	cmd := exec.Command("ffprobe", "-v", "quiet", "-print_format", "json", "-show_streams", *path)
	outputBytes, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Error executing ffprobe command: %s", err)
	}

	// Step 2: Parse JSON output from ffprobe
	var probe ProbeData
	if err := json.Unmarshal(outputBytes, &probe); err != nil {
		log.Fatalf("Error parsing ffprobe output: %s", err)
	}

	// Step 3: Find the audio track with Japanese language and the French subtitle track
	log.Println("Searching for Audio track (Japanese) and Subtitle track (French)")
	audioTrackID := -1
	subtitleTrackID := -1
	var totalDuration int // total duration in seconds

	for _, stream := range probe.Streams {
		if stream.CodecType == "audio" && stream.Tags.Language == "jpn" {
			log.Printf("Found Audio track ID: %d (Japanese)", stream.Index)
			audioTrackID = stream.Index
		}
		if stream.CodecType == "subtitle" && stream.Tags.Language == "fre" {
			log.Printf("Found Subtitle track ID: %d (French)", stream.Index)
			subtitleTrackID = stream.Index
		}
		// Get total duration from the first audio or video stream found
		if stream.Tags.Duration != "" {
			totalDuration, err = parseTimeToSeconds(stream.Tags.Duration)
			if err != nil {
				log.Fatalf("Error parsing duration: %s", err)
			}
		}
	}

	if audioTrackID == -1 {
		log.Fatalf("Error: No Japanese audio track found.")
	}
	if subtitleTrackID == -1 {
		log.Fatalf("Error: No French subtitle track found.")
	}

	// Step 4: Process the video using the selected audio track and hardcode the subtitles
	log.Println("Processing video with the selected audio track and hardcoded subtitles")
	log.Printf("Output file path: %s", *output)
	progressFile, err := os.CreateTemp("", "ffmpeg_progress_*.txt")
	if err != nil {
		log.Fatalf("Error creating temp file: %s", err)
	}
	defer os.Remove(progressFile.Name()) // Clean up the temp file after use

	ffmpegCmd := exec.Command(
		"ffmpeg",
		"-i", *path, // Input file
		"-y",          // Overwrite output file without asking
		"-map", "0:v", // Map video stream
		"-map", fmt.Sprintf("0:%d", audioTrackID), // Map audio stream
		"-vf", fmt.Sprintf("subtitles=%s", *path), // Hardcode subtitles
		"-c:v", "h264_nvenc", // Use NVIDIA NVENC for video encoding
		"-crf", "28", // Quality
		"-preset", "slow", // Preset for encoding speed
		"-c:a", "aac", // Audio codec
		"-b:a", "128k", // Audio bitrate
		"-progress", progressFile.Name(), // Progress file
		*output, // Output file
	)

	// Print the command for debugging
	log.Printf("Executing command: %s", strings.Join(ffmpegCmd.Args, " "))

	// Start the command
	if err := ffmpegCmd.Start(); err != nil {
		log.Fatalf("Error starting ffmpeg command: %s", err)
	}

	// Setup a progress bar
	bar := progressbar.NewOptions(totalDuration, progressbar.OptionSetDescription("Processing video..."), progressbar.OptionShowCount())

	// Process ffmpeg output to update the progress bar
	go func() {
		for {
			// Open the progress file
			f, err := os.Open(progressFile.Name())
			if err != nil {
				log.Printf("Error opening progress file: %s", err)
				return
			}

			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.Contains(line, "out_time=") {
					// Extract the out_time value from the line
					parts := strings.Split(line, "=")
					if len(parts) == 2 {
						timeValue := strings.TrimSpace(parts[1])            // Trim any whitespace
						actualSeconds, err := parseTimeToSeconds(timeValue) // Use the new parse function
						if err != nil {
							log.Printf("Error parsing time: %s", err)
							continue
						}

						bar.Set(actualSeconds) // Update the progress bar
					}
				}
			}
			f.Close()

			// Wait a bit before the next read
			time.Sleep(1 * time.Second)
		}
	}()

	if err := ffmpegCmd.Wait(); err != nil {
		log.Fatalf("Error during video processing: %s", err)
	}

	log.Println("Video processing completed successfully")
}

func parseTimeToSeconds(timeStr string) (int, error) {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid time format")
	}

	hours, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("error parsing hours: %w", err)
	}

	minutes, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("error parsing minutes: %w", err)
	}

	secondsParts := strings.Split(parts[2], ".")
	seconds, err := strconv.Atoi(secondsParts[0])
	if err != nil {
		return 0, fmt.Errorf("error parsing seconds: %w", err)
	}

	totalSeconds := hours*3600 + minutes*60 + seconds

	// Handle fractional seconds if present
	if len(secondsParts) > 1 {
		fractionalPart, err := strconv.Atoi(secondsParts[1])
		if err == nil {
			totalSeconds += int(float64(fractionalPart) * 1e-6) // Convert microseconds to seconds
		}
	}

	return totalSeconds, nil
}
