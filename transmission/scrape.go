package transmission

import (
	"context"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/anacrolix/log"
	"github.com/madflojo/tasks"
)

// Summary of Transmission Announcer Scrape Behavior:
//
// 1. Scrape Triggering:
//    - Scrapes are initiated during a periodic upkeep cycle (runs every 500ms).
//    - A specific tracker tier is marked for scraping when its scheduled 'scrapeAt'
//      time is reached or passed, and it's not already scraping.
//    - The 'scrapeAt' time is determined by:
//        - The interval provided in the last successful scrape response.
//        - A default interval (DefaultScrapeIntervalSec = 1800 seconds, i.e., 30
//          minutes) if no interval was provided.
//        - Calculated retry intervals after failed scrape attempts.
//        - An immediate schedule ('scrapeSoon') upon tier initialization.
//
// 2. Initial Scrape Timing:
//    - The very first scrape for a newly added torrent/tracker tier is scheduled
//      immediately upon its creation (via 'scrapeSoon').
//    - This initial scrape runs concurrently with, or very close in time to,
//      the initial 'started' announce request. It does *not* wait for the
//      initial announce to complete.
//
// 3. Rate Limiting:
//    - MaxScrapesPerUpkeep: A maximum of 20 distinct scrape *requests* (batches)
//      can be initiated within a single 500ms upkeep cycle. This limits the
//      overall request rate across all trackers.
//    - TrMultiscrapeMax: A single scrape request to a specific tracker URL can
//      initially contain up to 60 torrent infohashes (multiscrape). I actually
//      never see batch request in Transmission, just ignore this and the following
//      TrMultiscrapeStep.
//    - TrMultiscrapeStep: If a tracker responds with an error indicating the
//      request was too large (e.g., "Request-URI Too Long"), the maximum number
//      of infohashes allowed for *that specific tracker's* future requests
//      (its 'multiscrape_max') is reduced by 5. This allows dynamic adaptation
//      to individual tracker limits. Ignore this.
//    - Scheduling Intervals: Scrapes for a given tracker only occur after the
//      specified 'scrapeIntervalSec' or retry interval has elapsed, preventing
//      constant scraping of the same tracker.
//
// How to mimick scrape request in go anacrolix/torrent?
//
// - in mimickTransmission, when adding new perTorrent, it should send a scrape
//   request, and schedule a delayed task to keep sending requests.
// - delayed task should store info_hash and peer_id, when task run, check the if
//   mimickTransmission.torrents is still storing the same perTorrent, if not just
//   don't run.
// - delayed task can just use 30 min interval for next run. use container/list +
//   lock to impl.
// - delayed task should store the scheduled time, scheduler can pop task from list
//   if it passed scheduled time, run it, or sleep until (min 0.5s). in each uptake
//   runner should not process more than 20 tasks.
// - result of scrape request can be just ignored, we don't use it.

var (
	httpClient = http.DefaultClient
)

const (
	// Max 40 scrape requests per second.
	maxScrapesPerSecond = 40

	// Default interval 30 min.
	scrapeInterval = 30 * time.Minute
)

// scrapeTask holds information needed for a scheduled scrape.
type scrapeTask struct {
	tr        *mimickTransmission
	scrapeURL *url.URL
}

func newScrapeTask(tr *mimickTransmission, announceURL *url.URL, infoHash string, privateTrackerQuery string) *scrapeTask {
	u := scrapeURL(announceURL, infoHash, privateTrackerQuery)
	if u == nil {
		return nil
	}

	return &scrapeTask{
		tr:        tr,
		scrapeURL: u,
	}
}

func scrapeURL(announceURL *url.URL, infoHash, privateTrackerQuery string) *url.URL {
	// path does not ending with /announce means this tracker does not support scrape.
	if !strings.HasSuffix(announceURL.Path, "/announce") {
		return nil
	}
	scrapeURL := announceURL.JoinPath("../scrape")

	query := url.Values{}
	query.Add("info_hash", infoHash)
	infoHashQuery := query.Encode()
	if privateTrackerQuery != "" {
		scrapeURL.RawQuery = privateTrackerQuery + "&" + infoHashQuery
	} else {
		scrapeURL.RawQuery = infoHashQuery
	}

	return scrapeURL
}

func (t *scrapeTask) run() {
	err := t.tr.scrapeRateLimiter.Wait(context.Background())
	if err != nil {
		logger.Levelf(log.Error, "Request failed to acquire token %v", err)
		return
	}

	finalURL := t.scrapeURL.String()

	req, err := http.NewRequest("GET", finalURL, nil)
	if err != nil {
		logger.Levelf(log.Error, "Failed to create scrape request for %s: %v", finalURL, err)
		return
	}

	req.Header.Set("User-Agent", "Transmission/4.0.6")
	req.Header.Set("Accept-Encoding", "deflate, gzip, br, zstd")
	req.Header.Set("Accept", "*/*")

	resp, err := httpClient.Do(req)
	if err != nil {
		logger.Levelf(log.Info, "Scrape request failed for %s: %v", finalURL, err)
		return
	}
	resp.Body.Close()
}

func (s *mimickTransmission) scheduleScrape(id string, task *scrapeTask) {
	if task == nil {
		return
	}
	s.scheduler.AddWithID(id, &tasks.Task{
		Interval: scrapeInterval,
		// add some random delay to avoid batch added torrents blocking on rate limiter.
		StartAfter: time.Now().Add(time.Duration(rand.Int64N(9*1000)+1000) * time.Millisecond),
		TaskFunc: func() error {
			task.run()
			return nil
		},
	})
}
