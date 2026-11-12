package entity

type TMDBNameRequest struct {
	TMDBID string `json:"tmdbId"`
	Title  string `json:"title"`
	Ova    bool   `json:"ova"`
}

type TMDBNameResponse struct {
	ThemoviedbName string `json:"themoviedbName"`
	TMDB           *Tmdb  `json:"tmdb"`
}

type TMDBGroup struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Type         int    `json:"type"`
	TypeName     string `json:"typeName"`
	GroupCount   int    `json:"groupCount"`
	EpisodeCount int    `json:"episodeCount"`
}
