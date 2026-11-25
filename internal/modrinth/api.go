package modrinth

import (
	"encoding/json"
	"time"
)

type apiTime struct {
	time.Time
}

func (t *apiTime) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		t.Time = time.Time{}
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if s == "" {
		t.Time = time.Time{}
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return err
	}
	t.Time = parsed
	return nil
}

func (t *apiTime) Valid() bool {
	return !t.Time.IsZero()
}

type searchResults struct {
	Hits        []searchHit `json:"hits"`
	Page        int         `json:"page"`
	HitsPerPage int         `json:"hits_per_page"`
	TotalHits   int         `json:"total_hits"`
}

type searchHit struct {
	ProjectID           string                      `json:"project_id"`
	Slug                *string                     `json:"slug"`
	ProjectTypes        []string                    `json:"project_types"`
	AllProjectTypes     []string                    `json:"all_project_types"`
	AuthorID            *string                     `json:"author_id"`
	OrganizationID      *string                     `json:"organization_id"`
	DisclosureTypes     []string                    `json:"disclosure_types"`
	MinecraftServer     *serverProjectComponent     `json:"minecraft_server"`
	MinecraftJavaServer *javaServerProjectComponent `json:"minecraft_java_server"`
}

type apiProject struct {
	ID                  string                      `json:"id"`
	Slug                *string                     `json:"slug"`
	Name                string                      `json:"name"`
	TeamID              string                      `json:"team_id"`
	Org                 *string                     `json:"organization"`
	Published           apiTime                     `json:"published"`
	Approved            *apiTime                    `json:"approved"`
	Queued              *apiTime                    `json:"queued"`
	Status              string                      `json:"status"`
	ReqStatus           *string                     `json:"requested_status"`
	License             apiLicense                  `json:"license"`
	Downloads           int64                       `json:"downloads"`
	Followers           int                         `json:"followers"`
	ProjectTypes        []string                    `json:"project_types"`
	Categories          []string                    `json:"categories"`
	AddlCats            []string                    `json:"additional_categories"`
	IconURL             *string                     `json:"icon_url"`
	Color               *int                        `json:"color"`
	Monetization        string                      `json:"monetization_status"`
	SideTypes           string                      `json:"side_types_migration_review_status"`
	LinkURLs            map[string]apiLink          `json:"link_urls"`
	Gallery             []apiGalleryItem            `json:"gallery"`
	Versions            []string                    `json:"versions"`
	MinecraftServer     *serverProjectComponent     `json:"minecraft_server"`
	MinecraftJavaServer *javaServerProjectComponent `json:"minecraft_java_server"`
}

type apiLicense struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	URL  *string `json:"url"`
}

type apiLicenseRef struct {
	Short string `json:"short"`
	Name  string `json:"name"`
}

type apiLink struct {
	Platform string `json:"platform"`
	Donation bool   `json:"donation"`
	URL      string `json:"url"`
}

type apiGalleryItem struct {
	URL string `json:"url"`
}

type serverProjectComponent struct {
	MaxPlayers    *int     `json:"max_players"`
	Country       *string  `json:"country"`
	Region        *string  `json:"region"`
	Languages     []string `json:"languages"`
	ActiveVersion *string  `json:"active_version"`
}

type javaServerProjectComponent struct {
	Address         string          `json:"address"`
	Content         serverContent   `json:"content"`
	Ping            *javaServerPing `json:"ping"`
	VerifiedPlays2w *uint64         `json:"verified_plays_2w"`
	VerifiedPlays4w *uint64         `json:"verified_plays_4w"`
}

type serverContent struct {
	Kind                   string   `json:"kind"`
	VersionID              *string  `json:"version_id"`
	ProjectID              *string  `json:"project_id"`
	SupportedGameVersions  []string `json:"supported_game_versions"`
	RecommendedGameVersion *string  `json:"recommended_game_version"`
}

type javaServerPing struct {
	When apiTime             `json:"when"`
	Data *javaServerPingData `json:"data"`
}

type javaServerPingData struct {
	Latency         pingDuration `json:"latency"`
	VersionName     string       `json:"version_name"`
	VersionProtocol int          `json:"version_protocol"`
	PlayersOnline   int          `json:"players_online"`
	PlayersMax      int          `json:"players_max"`
}
type pingDuration struct {
	Secs  int64 `json:"secs"`
	Nanos int64 `json:"nanos"`
}

func (d pingDuration) Duration() time.Duration {
	return time.Duration(d.Secs)*time.Second + time.Duration(d.Nanos)*time.Nanosecond
}

type apiVersion struct {
	ID            string          `json:"id"`
	ProjectID     string          `json:"project_id"`
	AuthorID      string          `json:"author_id"`
	Featured      bool            `json:"featured"`
	Name          *string         `json:"name"`
	VersionNumber string          `json:"version_number"`
	ProjectTypes  []string        `json:"project_types"`
	Games         []string        `json:"games"`
	DatePublished apiTime         `json:"date_published"`
	Downloads     int             `json:"downloads"`
	VersionType   string          `json:"version_type"`
	Status        string          `json:"status"`
	Files         []apiFile       `json:"files"`
	Dependencies  []apiDependency `json:"dependencies"`
	Loaders       []string        `json:"loaders"`
	GameVersions  []string        `json:"game_versions"`
	Environment   *string         `json:"environment"`
}

type apiFile struct {
	ID       string            `json:"id"`
	Filename string            `json:"filename"`
	Hashes   map[string]string `json:"hashes"`
	Primary  bool              `json:"primary"`
	Size     int64             `json:"size"`
}

type apiDependency struct {
	VersionID *string `json:"version_id"`
	ProjectID *string `json:"project_id"`
	FileName  *string `json:"file_name"`
	Type      string  `json:"dependency_type"`
}

type apiTeamMember struct {
	TeamID  string  `json:"team_id"`
	User    apiUser `json:"user"`
	Role    string  `json:"role"`
	IsOwner bool    `json:"is_owner"`
}

type apiUser struct {
	ID       string  `json:"id"`
	Username string  `json:"username"`
	Created  apiTime `json:"created"`
	Role     string  `json:"role"`
	Badges   uint64  `json:"badges"`
}

type apiOrganization struct {
	ID      string          `json:"id"`
	Slug    string          `json:"slug"`
	Name    string          `json:"name"`
	TeamID  string          `json:"team_id"`
	Color   *int            `json:"color"`
	Members []apiTeamMember `json:"members"`
}

type apiCategory struct {
	Name        string `json:"name"`
	ProjectType string `json:"project_type"`
	Header      string `json:"header"`
}

type apiLoader struct {
	Name                  string   `json:"name"`
	SupportedProjectTypes []string `json:"supported_project_types"`
}

type apiLinkPlatform struct {
	Name     string `json:"name"`
	Donation bool   `json:"donation"`
}

type apiGameVersion struct {
	Value   string  `json:"value"`
	Created apiTime `json:"created"`
	Type    *string `json:"type"`
	Major   *bool   `json:"major"`
}

type apiStatistics struct {
	Projects *int64 `json:"projects"`
	Versions *int64 `json:"versions"`
	Authors  *int64 `json:"authors"`
	Files    *int64 `json:"files"`
}

type apiRevenue struct {
	AllTime          string          `json:"all_time"`
	AllTimeAvailable string          `json:"all_time_available"`
	Data             []apiRevenueRow `json:"data"`
}

type apiRevenueRow struct {
	Time           int64  `json:"time"`
	Revenue        string `json:"revenue"`
	CreatorRevenue string `json:"creator_revenue"`
}

func jsonList(ids []string) string {
	out, _ := json.Marshal(ids)
	return string(out)
}
