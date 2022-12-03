package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"text/template"
	"time"

	"github.com/gookit/config/v2"
	"github.com/gookit/config/v2/toml"
	"github.com/gookit/goutil/maputil"
	"github.com/gookit/goutil/strutil"
	"github.com/nicklaw5/helix/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/rs/zerolog/pkgerrors"
)

type H = map[string]any

type State struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`

	GameName string `json:"-"`
	Title    string `json:"-"`
}

func (s *State) Load() error {
	data, err := os.ReadFile("state.json")

	if err != nil {
		return nil
	}

	if err := json.Unmarshal(data, s); err != nil {
		return err
	}

	return nil
}

func (s *State) Save() error {
	data, err := json.Marshal(s)

	if err != nil {
		return err
	}

	return os.WriteFile("state.json", data, 0666)
}

const (
	vlcPassword = "Popcorn"
)

var (
	titleTemplate *template.Template

	state State
)

func openURL(url string) error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("xdg-open", url).
			Start()

	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).
			Start()

	case "darwin":
		return exec.Command("open", url).
			Start()
	}

	return fmt.Errorf("cannot open url %s on this platform", url)
}

func openVLC() error {
	return exec.Command("vlc", "--extraintf", "http", "--http-password", vlcPassword).
		Start()
}

func formatTemplate(t *template.Template, data any) (string, error) {
	builder := new(strings.Builder)

	if err := t.Execute(builder, data); err != nil {
		return "", err
	}

	return builder.String(), nil
}

func fetchPlayerStatus() (H, error) {
	log.Debug().
		Msg("Fetching player status...")

	req, err := http.NewRequest("GET", "http://localhost:8080/requests/status.json", nil)

	if err != nil {
		return nil, err
	}

	req.SetBasicAuth("", vlcPassword)

	res, err := http.DefaultClient.Do(req)

	if err != nil {
		return nil, err
	}

	var result H

	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, err
	}

	log.Debug().
		Msg("Fetched player status")

	return result, nil
}

func checkAccessToken(client *helix.Client) (string, error) {
	isValid, validateResp, err := client.ValidateToken(client.GetUserAccessToken())

	if err != nil {
		return "", err
	}

	if !isValid {
		resp, err := client.RefreshUserAccessToken(state.RefreshToken)

		if err != nil {
			return "", err
		}

		state.AccessToken = resp.Data.AccessToken
		state.RefreshToken = resp.Data.RefreshToken

		if err := state.Save(); err != nil {
			return "", err
		}

		client.SetUserAccessToken(state.AccessToken)
	}

	return validateResp.Data.UserID, nil
}

func updateChannelInformation(client *helix.Client, broadcasterID string) error {
	log.Debug().
		Msg("Updating channel information...")

	resp, err := client.GetGames(&helix.GamesParams{
		Names: []string{state.GameName},
	})

	if err != nil {
		return err
	}

	gameID := ""

	if len(resp.Data.Games) > 0 {
		gameID = resp.Data.Games[0].ID
	}

	title, err := formatTemplate(titleTemplate, state)

	if err != nil {
		return err
	}

	_, err = client.EditChannelInformation(&helix.EditChannelInformationParams{
		BroadcasterID: broadcasterID,
		GameID:        gameID,
		Title:         title,
	})

	if err != nil {
		return err
	}

	log.Debug().
		Msg("Refreshed channel information")

	return nil
}

func refresh(client *helix.Client) error {
	data, err := fetchPlayerStatus()

	if err != nil {
		return err
	}

	ps, _ := strutil.String(maputil.DeepGet(data, "state"))

	if ps == "playing" {
		gameName, _ := strutil.String(maputil.DeepGet(data, "information.category.meta.album"))
		title, _ := strutil.String(maputil.DeepGet(data, "information.category.meta.title"))

		if state.Title == title {
			return nil
		}

		userID, err := checkAccessToken(client)

		if err != nil {
			return err
		}

		state.GameName = gameName
		state.Title = title

		if err := updateChannelInformation(client, userID); err != nil {
			return err
		}
	}

	return nil
}

func authorize(client *helix.Client) error {
	listener, err := net.Listen("tcp", "localhost:21825")

	if err != nil {
		return err
	}

	url := client.GetAuthorizationURL(&helix.AuthorizationURLParams{
		ResponseType: "code",
		Scopes: []string{
			"channel:manage:broadcast",
		},
	})

	if err := openURL(url); err != nil {
		return err
	}

	return http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		listener.Close()

		switch r.URL.Path {
		case "/":
			code := r.URL.Query().
				Get("code")

			resp, err := client.RequestUserAccessToken(code)

			if err != nil {
				return
			}

			state.AccessToken = resp.Data.AccessToken
			state.RefreshToken = resp.Data.RefreshToken

			if err := state.Save(); err != nil {
				return
			}

			client.SetUserAccessToken(state.AccessToken)

			w.Header().
				Set("Content-Type", "text/plain")

			w.Write([]byte("Connected to Twitch, you can close this page."))
		}
	}))
}

func main() {
	config.AddDriver(toml.Driver)

	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out: os.Stdout,
	})

	if err := state.Load(); err != nil {
		log.Fatal().Err(err).
			Msg("An error occured while loading the state")
	}

	if err := config.LoadFiles("config.toml"); err != nil {
		log.Fatal().Err(err).
			Msg("An error occured while loading the configuration file")
	}

	t, err := template.New("title").
		Parse(config.String("title"))

	if err != nil {
		log.Fatal().Err(err).
			Msg("An error occured while parsing the title template")
	}

	titleTemplate = t

	client, err := helix.NewClient(&helix.Options{
		ClientID:     config.String("twitch.client_id"),
		ClientSecret: config.String("twitch.client_secret"),

		RedirectURI:     "http://localhost:21825",
		UserAccessToken: state.AccessToken,
	})

	if state.AccessToken == "" {
		authorize(client)
	}

	if err != nil {
		log.Fatal().Err(err).
			Msg("An error occured while creating the Twitch client")
	}

	if err := openVLC(); err != nil {
		log.Fatal().Err(err).
			Msg("An error occured while opening VLC")
	}

	for {
		if err := refresh(client); err != nil {
			log.Error().Err(err).
				Msg("An error occured while refreshing")
		}

		time.Sleep(time.Minute)
	}
}
