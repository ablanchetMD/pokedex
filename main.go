package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ablanchetMD/pokedex/pokecache"
)

type cliCommand struct {
	name        string
	description string
	callback    func(params ...string) error
}

type pokemonEntry struct {
	createdAt time.Time
	data      []byte
}

type pokedex struct {
	entries map[string]pokemonEntry
	mu      sync.Mutex
}

func (p *pokedex) Add(key string, data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.entries[key] = pokemonEntry{
		createdAt: time.Now(),
		data:      data,
	}
	return nil
}

func (p *pokedex) Get(key string) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.entries[key]
	if !ok {
		return nil, errors.New("key not found")
	}
	return entry.data, nil
}

func NewPokedex() *pokedex {
	p := &pokedex{
		entries: make(map[string]pokemonEntry),
	}

	return p
}

var pCache *pokecache.Cache
var pDex *pokedex
var commands map[string]cliCommand

func init() {
	api := &PokeAPI{}
	nextURL := "https://pokeapi.co/api/v2/location-area"
	api.NextURL = &nextURL
	pCache = pokecache.NewCache()
	pDex = NewPokedex()
	commands = make(map[string]cliCommand)
	commands["help"] = cliCommand{
		name:        "help",
		description: "Displays a help message",
		callback:    commandHelp,
	}
	commands["exit"] = cliCommand{
		name:        "exit",
		description: "Exits the Pokedex",
		callback:    commandExit,
	}
	commands["map"] = cliCommand{
		name:        "map",
		description: "Get the next 20 results from the location API",
		callback: func(params ...string) error {
			return api.commandMap("next")
		},
	}
	commands["mapb"] = cliCommand{
		name:        "mapb",
		description: "Get the previous 20 results from the location API",
		callback: func(params ...string) error {
			return api.commandMap("prev")
		},
	}
	commands["explore"] = cliCommand{
		name:        "explore",
		description: "Explore <location> to find Pokemon, with <location> being the name or id of the location",
		callback:    commandExplore,
	}
	commands["catch"] = cliCommand{
		name:        "catch",
		description: "Try to catch <pokemon>, with <pokemon> being the name or id of the pokemon you are trying to catch.",
		callback:    commandCatch,
	}

	commands["inspect"] = cliCommand{
		name:        "inspect",
		description: "Inspect the following <pokemon>, with <pokemon> being the name or id of the pokemon you are trying to inspect. You can only inspect a pokemon you have caught.",
		callback:    commandInspect,
	}

	commands["pokedex"] = cliCommand{
		name:        "pokedex",
		description: "Displays a list of all pokemons you have caught.",
		callback:    commandPokedex,
	}

}

type PokeLoc struct {
	Count    int     `json:"count"`
	Next     *string `json:"next"`
	Previous *string `json:"previous"`
	Results  []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"results"`
}

type PokeLocal struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	PokemonEncounters []struct {
		Pokemon struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"pokemon"`
	} `json:"pokemon_encounters"`
}

type Pokemon struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	BaseExperience int    `json:"base_experience"`
	Height         int    `json:"height"`
	IsDefault      bool   `json:"is_default"`
	Order          int    `json:"order"`
	Weight         int    `json:"weight"`
	Abilities      []struct {
		IsHidden bool `json:"is_hidden"`
		Slot     int  `json:"slot"`
		Ability  struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"ability"`
	} `json:"abilities"`
	Forms []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"forms"`
	LocationAreaEncounters string `json:"location_area_encounters"`
	Moves                  []struct {
		Move struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"move"`
		VersionGroupDetails []struct {
			LevelLearnedAt int `json:"level_learned_at"`
			VersionGroup   struct {
				Name string `json:"name"`
				URL  string `json:"url"`
			} `json:"version_group"`
			MoveLearnMethod struct {
				Name string `json:"name"`
				URL  string `json:"url"`
			} `json:"move_learn_method"`
		} `json:"version_group_details"`
	} `json:"moves"`
	Species struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"species"`
	Cries struct {
		Latest string `json:"latest"`
		Legacy string `json:"legacy"`
	} `json:"cries"`
	Stats []struct {
		BaseStat int `json:"base_stat"`
		Effort   int `json:"effort"`
		Stat     struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"stat"`
	} `json:"stats"`
	Types []struct {
		Slot int `json:"slot"`
		Type struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"type"`
	} `json:"types"`
}

type PokeAPI struct {
	NextURL *string
	PrevURL *string
}

func commandExit(params ...string) error {
	os.Exit(0)
	return nil
}

func commandExplore(params ...string) error {
	if len(params) < 1 {
		fmt.Println("Please provide a location name")
		return errors.New("no location name provided")
	}

	fmt.Println("Exploring location:", params[0])
	var resp *http.Response
	var err error
	url := "https://pokeapi.co/api/v2/location-area/" + params[0]

	// Check the cache
	data, err := pCache.Get(url)
	if err == nil {
		// Cache hit
		return processExplore(data)
	}

	resp, err = http.Get(url)
	if err != nil {
		fmt.Println("Error fetching data:", err)
		return err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response:", err)
		return err
	}
	// Check if the content type is JSON
	if contentType := resp.Header.Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		fmt.Println("Response is not JSON:", contentType)
		return errors.New("response is not JSON")
	}
	// Cache the response body
	err = pCache.Add(url, body)
	if err != nil {
		fmt.Println("Error adding to cache:", err)
		return err
	}

	// Process the response body
	return processExplore(body)

}

func processExplore(data []byte) error {
	var locs PokeLocal

	err := json.Unmarshal(data, &locs)
	if err != nil {
		fmt.Println("Error unmarshalling JSON:", err)
		return err
	}

	// Print the struct to verify
	if len(locs.PokemonEncounters) > 0 {
		fmt.Println("Pokemon found:")
	}
	for _, loc := range locs.PokemonEncounters {
		fmt.Println(loc.Pokemon.Name)
	}

	return nil
}

func (api *PokeAPI) commandMap(dir string) error {
	var resp *http.Response
	var err error
	var url string
	if dir == "next" {
		if api.NextURL == nil {
			fmt.Println("No more results")
			return nil
		}
		url = *api.NextURL
	} else if dir == "prev" {
		if api.PrevURL == nil {
			fmt.Println("No more results")
			return nil
		}
		url = *api.PrevURL
	} else {
		fmt.Println("Invalid direction")
		return errors.New("invalid direction")
	}
	// Check the cache
	data, err := pCache.Get(url)
	if err == nil {
		// Cache hit
		return processResponse(data, api)
	}

	resp, err = http.Get(url)
	if err != nil {
		fmt.Println("Error fetching data:", err)
		return err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response:", err)
		return err
	}
	// Check if the content type is JSON
	if contentType := resp.Header.Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		fmt.Println("Response is not JSON:", contentType)
		return errors.New("response is not JSON")
	}
	// Cache the response body
	err = pCache.Add(url, body)
	if err != nil {
		fmt.Println("Error adding to cache:", err)
		return err
	}

	// Process the response body
	return processResponse(body, api)
}

func processResponse(data []byte, api *PokeAPI) error {
	var locs PokeLoc

	err := json.Unmarshal(data, &locs)
	if err != nil {
		fmt.Println("Error unmarshalling JSON:", err)
		return err
	}

	// Print the struct to verify
	for _, loc := range locs.Results {
		fmt.Println(loc.Name)
	}

	api.NextURL = locs.Next
	api.PrevURL = locs.Previous

	return nil
}

func commandHelp(params ...string) error {
	keys := make([]string, 0, len(commands))
	for k := range commands {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Println("Welcome to the Pokedex!")
	fmt.Println()
	fmt.Println("Available commands:")
	fmt.Println()
	for _, k := range keys {
		fmt.Printf("%s: %s\n", k, commands[k].description)
	}
	return nil
}

func commandPokedex(params ...string) error {
	fmt.Println("Pokedex:")
	for name := range pDex.entries {
		fmt.Println("  -", name)
	}
	return nil
}

func commandInspect(params ...string) error {
	if len(params) < 1 {
		fmt.Println("Please provide a Pokemon name")
		return errors.New("no Pokemon name provided")
	}
	data, err := pDex.Get(params[0])
	if err != nil {
		fmt.Println("You have not caught that pokemon yet (or there was an error):", params[0])
		return err
	}
	var pokemon Pokemon
	err = json.Unmarshal(data, &pokemon)
	if err != nil {
		fmt.Println("Error unmarshalling JSON:", err)
		return err
	}
	fmt.Printf("Name: %s\n", pokemon.Name)
	fmt.Printf("Height: %d\n", pokemon.Height)
	fmt.Printf("Weight: %d\n", pokemon.Weight)
	fmt.Println("Stats:")
	for _, stat := range pokemon.Stats {
		fmt.Printf("  -%s: %d\n", stat.Stat.Name, stat.BaseStat)
	}
	fmt.Println("Types:")
	for _, t := range pokemon.Types {
		fmt.Println("  - ", t.Type.Name)
	}
	return nil
}

func commandCatch(params ...string) error {
	if len(params) < 1 {
		fmt.Println("Please provide a Pokemon name")
		return errors.New("no Pokemon name provided")
	}
	var resp *http.Response
	var err error
	url := "https://pokeapi.co/api/v2/pokemon/" + params[0]

	// Check the cache
	data, err := pCache.Get(url)
	if err == nil {
		// Cache hit
		return processCatch(data)
	}

	resp, err = http.Get(url)
	if err != nil {
		fmt.Println("Error fetching data:", err)
		return err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response:", err)
		return err
	}
	// Check if the content type is JSON
	if contentType := resp.Header.Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		fmt.Println("Response is not JSON:", contentType)
		return errors.New("response is not JSON")
	}
	// Cache the response body
	err = pCache.Add(url, body)
	if err != nil {
		fmt.Println("Error adding to cache:", err)
		return err
	}

	// Process the response body
	return processCatch(body)
}

func processCatch(data []byte) error {
	var pokemon Pokemon

	err := json.Unmarshal(data, &pokemon)
	if err != nil {
		fmt.Println("Error unmarshalling JSON:", err)
		return err
	}

	// Print the struct to verify
	fmt.Printf("Throwing a Pokeball at %s...\n", pokemon.Name)
	dice := rand.Intn(10)
	if dice*pokemon.BaseExperience > 400 {
		fmt.Println("Oh no! The", pokemon.Name, "escaped!")
		fmt.Printf("Dice Roll : %d * %d > 400\n", dice, pokemon.BaseExperience)
	} else {
		fmt.Println("Gotcha! You caught a", pokemon.Name)
		pDex.Add(pokemon.Name, data)
	}

	return nil
}

// func loadPokedex(filename string) (map[string][]byte, error) {
//     file, err := os.Open(filename)
//     if err != nil {
//         if os.IsNotExist(err) {
//             return make(map[string][]byte), nil // Return an empty map if the file does not exist
//         }
//         return nil, err
//     }
//     defer file.Close()

//     var cache map[string][]byte
//     decoder := json.NewDecoder(file)
//     err = decoder.Decode(&cache)
//     if err != nil {
//         return nil, err
//     }

//     return cache, nil
// }

// func savePokedex(filename string, cache map[string][]byte) error {
//     file, err := os.Create(filename)
//     if err != nil {
//         return err
//     }
//     defer file.Close()

//     encoder := json.NewEncoder(file)
//     err = encoder.Encode(cache)
//     if err != nil {
//         return err
//     }

//     return nil
// }

func main() {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("Pokedex> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Error reading input:", err)
			return
		}
		input = strings.TrimSpace(input)
		parts := strings.Fields(input)

		if len(parts) == 0 {
			continue
		}
		command := parts[0]
		params := parts[1:]

		commandEntry, found := commands[command]

		if found {
			err := commandEntry.callback(params...)
			if err != nil {
				fmt.Println("Error executing command:", err)
			}
		} else {
			fmt.Println("Unknown command")
		}
	}
}
