package dto

type AuthUser struct {
	ID           string `json:"id" example:"usr_0123456789abcdef"`
	DiscordID    string `json:"discordId" example:"123456789012345678"`
	Name         string `json:"name" example:"Inspector General"`
	Avatar       string `json:"avatar,omitempty"`
	Role         string `json:"role" enums:"Owner,Screener,Admin,Guest"`
	IsAuthorized bool   `json:"isAuthorized" example:"true"`
}

type AuthToken struct {
	AccessToken string   `json:"accessToken"`
	TokenType   string   `json:"tokenType" example:"Bearer"`
	ExpiresIn   int64    `json:"expiresIn" example:"900"`
	User        AuthUser `json:"user"`
}
