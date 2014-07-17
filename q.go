package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"github.com/agentio/agent"
	"github.com/docopt/docopt-go"
	"github.com/olekukonko/tablewriter"
	"gopkg.in/yaml.v1"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func createAppArchive(appName string, zipFileName string) error {
	// create a new zip archive.
	buf, err := os.Create(zipFileName)
	w := zip.NewWriter(buf)

	// exclude anything matching this pattern from the archive
	exclusionPattern :=
		"(.DS_Store)" + "|" +
			"(go/.*/.git)" + "|" +
			"(go/.*/.bzr)" + "|" +
			"(go/.*/.hg)" + "|" +
			"(go/pkg)" + "|" +
			"(go/bin)"
	patternToExclude := regexp.MustCompile(exclusionPattern)

	// traverse the app directory, adding non-excluded files to the zip archive
	filepath.Walk(appName,
		func(path string, info os.FileInfo, err error) error {
			// skip excluded files
			if patternToExclude.MatchString(path) {
				//fmt.Printf("excluding %v\n", path)
			} else {
				fi, err := os.Open(path)
				defer fi.Close()
				fi_stat, err := fi.Stat()
				if err != nil {
					fmt.Println(err)
					return err
				}
				switch mode := fi_stat.Mode(); {
				case mode.IsRegular():
					// append regular files to the archive
					fo, err := w.Create(path)
					if err != nil {
						log.Fatal(err)
					}
					_, err = io.Copy(fo, fi)
				case mode.IsDir():
					// ignore directories, they will be implicit in the path names
				}
				if err != nil {
					log.Fatal(err)
				}
			}
			return err
		})

	err = w.Close()
	if err != nil {
		log.Fatal(err)
	}
	return err
}

func main() {
	usage := `The Agent Quartermaster.

Usage:
  q connect <service> [-u=<username> -p=<password>]
  q list
  q show <appid>
  q create <appname>
  q upload <appid>
  q (start|stop|restart) <appid>
  q (start|stop|restart) <appid> <versionid>
  q delete <appid>
  q delete <appid> <versionid>  
  q -h | --help
  q --version

Options:
  -h --help     Show this screen.
  --version     Show version.
  <service>		Agent URL
  -u=<username> Username
  -p=<password> Password
  <appid>       App identifier
  <versionid>   Version identifier`

	arguments, _ := docopt.Parse(usage, nil, true, "Agent I/O 0.1", false)

	credentialsFileName := fmt.Sprintf("%v/.agent.json", os.Getenv("HOME"))

	var c agent.Connection
	if arguments["connect"].(bool) {
		c.Service = arguments["<service>"].(string)
		c.Credentials = fmt.Sprintf("%v:%v", arguments["-u"], arguments["-p"])
		bytes, err := json.Marshal(c)
		check(err)
		err = ioutil.WriteFile(credentialsFileName, bytes, 0644)
		check(err)
	} else {
		bytes, err := ioutil.ReadFile(credentialsFileName)
		if err != nil {
			fmt.Printf("Unable to read agent information from %v\n", credentialsFileName)
			return
		}
		json.Unmarshal(bytes, &c)
	}

	if arguments["list"].(bool) {
		var apps []agent.App
		err := c.GetApps(&apps)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"Id", "Name", "Description", "Workers"})
		for _, app := range apps {
			table.Append([]string{
				app.Id.Hex(),
				app.Name,
				app.Description,
				strconv.Itoa(len(app.Workers))})
		}
		table.Render() // Send output

		return
	}

	if arguments["show"].(bool) {
		var app agent.App
		err := c.GetApp(&app, arguments["<appid>"].(string))
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
		{
			table := tablewriter.NewWriter(os.Stdout)
			table.SetColWidth(100)
			table.Append([]string{"Id", app.Id.Hex()})
			table.Append([]string{"Name", app.Name})
			table.Append([]string{"Description", app.Description})
			table.Append([]string{"Capacity", fmt.Sprintf("%v", app.Capacity)})
			table.Append([]string{"Paths", fmt.Sprintf("%v", app.Paths)})
			table.Append([]string{"Domains", fmt.Sprintf("%v", app.Domains)})
			table.Render()
		}

		if len(app.Versions) > 0 {
			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"Version", "Filename", "Created"})
			table.SetColWidth(100)
			for _, v := range app.Versions {
				table.Append([]string{v.Version, v.Filename, fmt.Sprintf("%v", v.Created)})
			}
			table.Render()
		}

		if len(app.Workers) > 0 {
			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"Container", "Host", "Port", "Version"})
			table.SetColWidth(100)
			for _, w := range app.Workers {
				table.Append([]string{w.Container, w.Host, fmt.Sprintf("%v", w.Port), w.Version})
			}
			table.Render()
		}

		return
	}

	if arguments["create"].(bool) {
		appname := arguments["<appname>"]
		bytes, err := ioutil.ReadFile(fmt.Sprintf("%v/app.yaml", appname))
		check(err)
		var appinfo agent.App
		yaml.Unmarshal(bytes, &appinfo)
		fmt.Printf("%v\n", appinfo)
		var result map[string]interface{}
		c.CreateApp(&result, appinfo)
		fmt.Printf("\n%+v\n\n", result)
		return
	}

	if arguments["upload"].(bool) {
		appid := arguments["<appid>"].(string)

		var app agent.App
		c.GetApp(&app, appid)

		var err error

		// create the zip file
		zipfilename := fmt.Sprintf("%v.zip", app.Name)

		os.Remove(zipfilename)

		err = createAppArchive(appid, zipfilename)

		bytes, err := ioutil.ReadFile(zipfilename)
		check(err)

		var result map[string]interface{}
		c.CreateAppVersion(&result, appid, bytes)
		fmt.Printf("\n%+v\n\n", result)

		return
	}

	if arguments["start"].(bool) {
		appid := arguments["<appid>"].(string)
		switch arguments["<versionid>"].(type) {
		case string:
			versionid := arguments["<versionid>"].(string)
			var result map[string]interface{}
			c.StartAppVersion(&result, appid, versionid)
			fmt.Printf("\n%+v\n\n", result)
		case nil:
			var result map[string]interface{}
			c.StartApp(&result, appid)
			fmt.Printf("\n%+v\n\n", result)
		default:
		}
		return
	}

	if arguments["stop"].(bool) {
		appid := arguments["<appid>"].(string)
		switch arguments["<versionid>"].(type) {
		case string:
			versionid := arguments["<versionid>"].(string)
			var result map[string]interface{}
			c.StopAppVersion(&result, appid, versionid)
			fmt.Printf("\n%+v\n\n", result)
		case nil:
			var result map[string]interface{}
			c.StopApp(&result, appid)
			fmt.Printf("\n%+v\n\n", result)
		default:
		}
		return
	}

	if arguments["restart"].(bool) {
		appid := arguments["<appid>"].(string)
		switch arguments["<versionid>"].(type) {
		case string:
			versionid := arguments["<versionid>"].(string)
			var result map[string]interface{}
			c.StopAppVersion(&result, appid, versionid)
			fmt.Printf("\n%+v\n\n", result)
			c.StartAppVersion(&result, appid, versionid)
			fmt.Printf("\n%+v\n\n", result)
		case nil:
			var result map[string]interface{}
			c.StopApp(&result, appid)
			fmt.Printf("\n%+v\n\n", result)
			c.StartApp(&result, appid)
			fmt.Printf("\n%+v\n\n", result)
		default:
		}
		return
	}

	if arguments["delete"].(bool) {
		appid := arguments["<appid>"].(string)
		switch arguments["<versionid>"].(type) {
		case string:
			versionid := arguments["<versionid>"].(string)
			var result map[string]interface{}
			c.DeleteAppVersion(&result, appid, versionid)
			fmt.Printf("\n%+v\n\n", result)
		case nil:
			var result map[string]interface{}
			c.DeleteApp(&result, appid)
			fmt.Printf("\n%+v\n\n", result)
		default:
		}
		return
	}

	fmt.Printf("\n%+v\n\n", arguments)
}
