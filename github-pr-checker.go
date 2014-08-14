package main

import (
	"github.com/google/go-github/github"
	"code.google.com/p/goauth2/oauth"
	"github.com/andybons/hipchat"
	"fmt"
	"time"
	"sort"
	"flag"
)

type ByCreatedAt []github.PullRequest
func (this ByCreatedAt) Len() int {
	return len(this)
}
func (this ByCreatedAt) Less(i, j int) bool {
	return this[i].CreatedAt.Before(*this[j].CreatedAt)
}
func (this ByCreatedAt) Swap(i, j int) {
	this[i], this[j] = this[j], this[i]
}

func main() {
	var orgName string; flag.StringVar(&orgName, "org", "", "Check repos owned by the specified GitHub organization")
	var hipchatRoomName string; flag.StringVar(&hipchatRoomName, "room", "", "Exclusively notify the specified HipChat room")
	var hipchatToken string; flag.StringVar(&hipchatToken, "hipchat-api-token", "", "HipChat API token used for notifications")
	var githubRepoToken string; flag.StringVar(&githubRepoToken, "repo-api-token", "", "GitHub API token with 'repo' scope")
	var githubHookToken string; flag.StringVar(&githubHookToken, "hook-api-token", "", "GitHub API token with 'read:repo_hook' scope")
	var ageThreshold int; flag.IntVar(&ageThreshold, "days", 3, "Number of days old the PR may be before considering it old")
	flag.Parse()

	if orgName == "" {
		fmt.Println("Please provide a valid value for org")
		return
	}
	if githubRepoToken == "" {
		fmt.Println("Please provide a valid value for repo-api-token")
		return
	}
	if githubHookToken == "" {
		fmt.Println("Please provide a valid value for hook-api-token")
		return
	}
	if hipchatToken == "" {
		fmt.Println("Please provide a valid value for hipchat-api-token")
		return
	}

	fmt.Println("searching for repos with HipChat hooks...")

	client := createClient(githubRepoToken)

	repos := make(chan github.Repository, 10)
	getReposDone := make(chan bool, 1)
	go getRepos(client, repos, getReposDone, orgName)

	confirmedRepos := make(chan github.Repository, 100)
	roomReposDone := make(chan bool, 1)
	hookReaderClient := createClient(githubHookToken)
	go roomRepos(hookReaderClient, repos, confirmedRepos, roomReposDone, hipchatRoomName)

	notifications := make(chan github.PullRequest, 100)
	notificationChecks := make(chan string, 100)
	notificationCheckCount := 0

	for {
		repo, ok := <-confirmedRepos
		if !ok {
			break //channel closed
		}
		notificationCheckCount++
		go func() {
			prs := getOpenPullRequests(client, repo)
			for _, pr := range prs {
				if pullRequestIsOld(pr, ageThreshold) {
					notifications <- pr
				}
			}
			notificationChecks <- *repo.Name
		}()
	}

	<-getReposDone
	<-roomReposDone

	fmt.Println("\nfinished fetching repos")

	for notificationCheckCount > 0 {
		notificationCheckCount--
		<-notificationChecks
	}

	fmt.Println("finished checking PRs")

	notificationsArray := make([]github.PullRequest, len(notifications))
	notificationIndex := 0
	for len(notifications) > 0 {
		notificationsArray[notificationIndex] = <-notifications
		notificationIndex++
	}

	fmt.Println("issuing", len(notificationsArray), "notifications")


	sort.Sort(ByCreatedAt(notificationsArray))

	for _, notification := range notificationsArray {
		notifyPullRequest(notification, hipchatRoomName, hipchatToken)
	}

	printRateLimit(client)
	printRateLimit(hookReaderClient)
}

func createClient(token string) github.Client {
	t := &oauth.Transport{
		Token: &oauth.Token{AccessToken: token},
	}
	return *github.NewClient(t.Client())
}

func getRepos(client github.Client, repos chan github.Repository, done chan bool, org string)  {
	opt := &github.RepositoryListByOrgOptions{Type: "all", ListOptions:github.ListOptions{Page:1}}
	for {
		reposPage, response, err := client.Repositories.ListByOrg(org, opt)
		for _, repo := range reposPage {
			repos <- repo
		}

		if err != nil {
			fmt.Println("Failed to fetch repos: ", err)
			done <- true
		}

		if (response.NextPage == 0) {
			break
		}

		opt.Page++
	}

	close(repos)
	done <- true
}

func roomRepos(client github.Client, repos chan github.Repository, confirmedRepos chan github.Repository, done chan bool, roomName string) {
	checkCount := 0
	checkCompleteChan := make(chan string, 10)
	for {
		repo, ok := <-repos
		if !ok {
			break // channel is closed
		}
		checkCount++
		go func() {
//			fmt.Print(".")
			hooks, _, err := client.Repositories.ListHooks(*repo.Owner.Login, *repo.Name, nil)
			if err != nil {
				fmt.Println("Failed to fetch hooks:", err)
				done <- true
			}
			for _, hook := range hooks {
				if *hook.Name == "hipchat" {
						if roomName == "" || hook.Config["room"] == roomName {
	//					fmt.Print("!")
						confirmedRepos <- repo
						break
					}
				}
			}
			checkCompleteChan <- *repo.Name
		}()
	}
	for i:=0; i<checkCount; i++ {
		<-checkCompleteChan
	}
	close(confirmedRepos)
	done <- true
}

func getOpenPullRequests(client github.Client, repo github.Repository) []github.PullRequest {
	fmt.Println("Checking PRs for:", *repo.Name)
	opts := github.PullRequestListOptions{State: "open"}
	prs ,_ , err := client.PullRequests.List(*repo.Owner.Login, *repo.Name, &opts)
	if err != nil {
		fmt.Println("Failed to fetch pull requests:", err)
		return []github.PullRequest{}
	}
	return prs
}

func pullRequestIsOld(pr github.PullRequest, ageThreshold int) bool {
	return pr.CreatedAt.Before(time.Now().Add(time.Duration(-24 * ageThreshold) * time.Hour))
}

func notifyPullRequest(pr github.PullRequest, room string, token string) {
	message := fmt.Sprintf("PR is %d days old: %s", int(time.Since(*pr.CreatedAt).Hours()/24), *pr.HTMLURL)
	client := hipchat.Client{AuthToken: token}
	req := hipchat.MessageRequest{
		RoomId:        room,
		From:          "PR Checker",
		Message:       message,
		Color:         hipchat.ColorRed,
		MessageFormat: hipchat.FormatText,
		Notify:        true,
	}
	fmt.Println("Sending HipChat notification:", message)
	if err := client.PostMessage(req); err != nil {
		fmt.Println("Failed to send HipChat notification:", err)
	}
}

func printRateLimit(client github.Client) {
	rate, _, err := client.RateLimit()
	if err != nil {
		fmt.Println("Error fetching rate limit:", err)
	} else {
		remaining := int(rate.Reset.Sub(time.Now()).Seconds())
		mins := remaining / 60
		secs := remaining % 60
		fmt.Printf("API Rate Limit: %d/%d remaining for %dm %ds \n\n", rate.Remaining, rate.Limit, mins, secs)
	}
}
