package plex

import (
	"log/slog"
	"net/http"
	"runtime"

	"github.com/coder/websocket"
)

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /{$}", s.handleRoot)
	m.HandleFunc("GET /identity", s.handleIdentity)
	m.HandleFunc("GET /media/providers", s.handleProviders)
	m.HandleFunc("GET /:/prefs", s.handlePrefs)
	m.HandleFunc("GET /updater/status", s.handleEmpty)
	m.HandleFunc("GET /activities", s.handleEmpty)
	m.HandleFunc("GET /:/websockets/notifications", s.handleNotifications)
	m.HandleFunc("GET /myplex/account", s.handleMyPlexAccount)
	m.HandleFunc("PUT /myplex/refreshReachability", s.handleOK)

	m.HandleFunc("GET /library/sections", s.handleSections)
	m.HandleFunc("GET /library/sections/{$}", s.handleSections)
	s.browseRoutes()
	s.playQueueRoutes()
	s.playbackRoutes()
	s.trickplayRoutes()
	s.searchRoutes()
	s.hubListRoutes()
	s.libMetaRoutes()
	s.playlistRoutes()
	s.streamRoutes()
	s.sessionRoutes()
	s.transcodeRoutes()
	s.collectionRoutes()
	s.loginRoutes()
}

// serverAttrs are the capability attributes PMS puts on / and
// /media/providers. myPlexSigninState is always "ok": even unclaimed, that
// keeps Plex Web from blocking every page with an "unclaimed server" notice.
func (s *Server) serverAttrs(e *Element) *Element {
	return e.
		A("allowCameraUpload", false).
		A("allowChannelAccess", false).
		A("allowMediaDeletion", false).
		A("allowSharing", false).
		A("allowSync", false).
		A("allowTuners", false).
		A("apiVersion", "0.2.0").
		A("backgroundProcessing", false).
		A("certificate", false).
		A("companionProxy", false).
		A("countryCode", "usa").
		A("eventStream", true).
		A("friendlyName", s.cfg.ServerName).
		A("livetv", 0).
		A("machineIdentifier", s.cfg.MachineID).
		A("musicAnalysis", 0).
		A("myPlex", false).
		A("myPlexMappingState", "unknown").
		A("myPlexSigninState", "ok").
		A("myPlexSubscription", false).
		A("offlineTranscode", 0).
		A("platform", platform()).
		A("platformVersion", runtime.Version()).
		A("pluginHost", false).
		A("pushNotifications", false).
		A("readOnlyLibraries", true).
		A("streamingBrainABRVersion", 3).
		A("streamingBrainVersion", 2).
		A("sync", false).
		A("transcoderActiveVideoSessions", s.transcodes.count()).
		A("transcoderAudio", true).
		A("transcoderLyrics", false).
		A("transcoderSubtitles", true).
		A("transcoderVideo", true).
		A("transcoderVideoBitrates", "64,96,208,320,720,1500,2000,3000,4000,8000,10000,12000,20000").
		A("transcoderVideoQualities", "0,1,2,3,4,5,6,7,8,9,10,11,12").
		A("transcoderVideoResolutions", "128,128,160,240,320,480,768,720,720,1080,1080,1080,1080").
		A("updatedAt", s.started.Unix()).
		A("updater", false).
		A("version", Version).
		A("voiceSearch", false)
}

func platform() string {
	switch runtime.GOOS {
	case "linux":
		return "Linux"
	case "darwin":
		return "MacOSX"
	case "windows":
		return "Windows"
	}
	return runtime.GOOS
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	mc := s.serverAttrs(Container())
	for _, key := range []string{"activities", "butler", "channels", "clients", "diagnostics", "hubs", "library", "livetv", "media", "metadata", "music", "neighborhood", "playQueues", "player", "playlists", "resources", "search", "server", "servers", "statistics", "system", "transcode", "updater", "user"} {
		mc.Add(E("Directory").A("count", 1).A("key", key).A("title", key))
	}
	write(w, r, mc)
}

func (s *Server) handleIdentity(w http.ResponseWriter, r *http.Request) {
	write(w, r, Container().
		A("size", 0).
		A("claimed", false).
		A("machineIdentifier", s.cfg.MachineID).
		A("version", Version))
}

func (s *Server) handleEmpty(w http.ResponseWriter, r *http.Request) {
	write(w, r, Container())
}

func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	secs, err := s.sections(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	content := E("Feature").A("key", "/library/sections").A("type", "content").
		Add(E("Directory").A("hubKey", "/hubs").A("title", "Home"))
	for _, sec := range secs {
		content.Add(s.sectionDirectory(sec, true))
	}
	// jellex is a per-user view of Jellyfin, so the library provider leaves
	// out the "manage" and "match" features, as PMS does for shared users.
	// Plex Web then hides metadata editing, matching and deletion.
	lib := E("MediaProvider").
		A("identifier", "com.plexapp.plugins.library").
		A("title", "Library").
		A("types", "video,audio,photo").
		A("protocols", "stream,download").
		Add(
			content,
			E("Feature").A("key", "/hubs/search").A("type", "search"),
			E("Feature").A("key", "/library/metadata").A("type", "metadata"),
			E("Feature").A("key", "/:/rate").A("type", "rate"),
			E("Feature").A("key", "/photo/:/transcode").A("type", "imagetranscoder"),
			E("Feature").A("key", "/hubs/promoted").A("type", "promoted"),
			E("Feature").A("key", "/hubs/continueWatching").A("type", "continuewatching"),
			E("Feature").A("key", "/actions").A("type", "actions").Add(
				E("Action").A("id", "removeFromContinueWatching").A("key", "/actions/removeFromContinueWatching"),
			),
			E("Feature").A("flavor", "universal").A("key", "/playlists").A("type", "playlist"),
			E("Feature").A("flavor", "universal").A("key", "/playQueues").A("type", "playqueue"),
			E("Feature").A("key", "/library/collections").A("type", "collection"),
			E("Feature").A("scrobbleKey", "/:/scrobble").A("unscrobbleKey", "/:/unscrobble").A("key", "/:/timeline").A("type", "timeline"),
			E("Feature").A("type", "queryParser"),
			E("Feature").A("key", "/library/search").A("type", "universalsearch"),
		)
	write(w, r, s.serverAttrs(Container()).Add(lib))
}

// handlePrefs returns the server settings Plex Web reads at startup. Values
// are fixed for now; nothing is persisted.
func (s *Server) handlePrefs(w http.ResponseWriter, r *http.Request) {
	setting := func(id, label, typ string, value any) *Element {
		return E("Setting").A("id", id).A("label", label).A("summary", "").A("type", typ).
			A("default", value).A("value", value).A("hidden", false).A("advanced", false).A("group", "general")
	}
	write(w, r, Container().Add(
		setting("FriendlyName", "Friendly name", "text", s.cfg.ServerName),
		setting("AcceptedEULA", "Accepted EULA", "bool", true),
		setting("PublishServerOnPlexOnlineKey", "Publish server on Plex Online", "bool", false),
		setting("ManualPortMappingMode", "Manually specify public port", "bool", false),
		setting("sendCrashReports", "Send crash reports to Plex", "bool", false),
		setting("FSEventLibraryUpdatesEnabled", "Scan my library automatically", "bool", false),
		setting("ButlerUpdateChannel", "Update channel", "text", "16"),
		setting("allowedNetworks", "List of IP addresses and networks that are allowed without auth", "text", "0.0.0.0/0"),
	))
}

// handleNotifications accepts the event websocket Plex clients keep open and
// pushes notifications (currently playback state) to it.
func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		slog.Debug("websocket accept", "err", err)
		return
	}
	defer c.CloseNow()
	s.pushNotifications(c.CloseRead(r.Context()), c)
}

// handleMyPlexAccount reports the server's plex.tv link, which jellex
// doesn't have (Plex Web still asks).
func (s *Server) handleMyPlexAccount(w http.ResponseWriter, r *http.Request) {
	writeRoot(w, r, E("MyPlex").
		A("signInState", "none").
		A("mappingState", "unknown").
		A("mappingError", "").
		A("subscriptionActive", false).
		A("subscriptionState", "Unknown"))
}

// handleOK acknowledges requests for server-side actions jellex has nothing
// to do for, such as refreshing remote-access reachability.
func (s *Server) handleOK(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
