package main

import (
	"flag"
	"fmt"
	"sort"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"golang.org/x/net/context"
)

type dockerImage struct {
	ID  string
	Tag string
}

type dockerImages []dockerImage

func (d dockerImages) Len() int      { return len(d) }
func (d dockerImages) Swap(i, j int) { d[i], d[j] = d[j], d[i] }
func (d dockerImages) Less(i, j int) bool {
	if d[i].Tag == "latest" {
		return true
	}
	if d[j].Tag == "latest" {
		return false
	}

	return d[i].Tag > d[j].Tag
}

const (
	defaultNum  int64  = 0
	defaultTime string = "cyvan"
	defaultName string = "cyvan"
)

const (
	TextRed = iota + 31
	TextGreen
	TextYellow
)

var (
	help          bool
	targetNum     int64
	targetTime    string
	targetImage   string
	isTrimByForce bool
)

func init() {

	flag.BoolVar(&help, "h", false, "Help")
	flag.Int64Var(&targetNum, "n", defaultNum, "Keep the most newest n images group by image repo")
	flag.StringVar(&targetTime, "t", defaultTime, "Keep the images which created after time t group by image repo")
	flag.StringVar(&targetImage, "i", defaultName, "Delete images like i, support regex like Hello* or *ll* etc.")
	flag.BoolVar(&isTrimByForce, "f", false, "Delete images by -f")

	flag.Usage = usage

}

func main() {

	flag.Parse()

	if help {
		flag.Usage()
		return
	}

	trimImages(targetNum, targetTime, targetImage, isTrimByForce)
}

// trim images by num, time, name with name has the highest priority, time has the second, num has the last
func trimImages(tNum int64, tTime, tName string, isForce bool) {
	// get client
	cli, err := conn()
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	isTrimByNum, isTrimByTime, isTrimByName := tNum != defaultNum, tTime != defaultTime, tName != defaultName

	// docker images prune
	if !isTrimByName && !isTrimByNum && !isTrimByTime {
		fmt.Print("Run docker image prune ... ")
		err := pruneImages(ctx, cli)
		if err != nil {
			text := fmt.Sprintf("Err %v", err)
			printlnWithColor(text, TextRed)
		} else {
			printlnWithColor("success", TextGreen)
		}

		return
	}

	images := groupImages(ctx, cli, isTrimByName, tName)

	idsForDel := filterImages(images, tNum, tTime, isTrimByNum, isTrimByTime)

	// docker rmi xxx
	cleanImages(ctx, cli, idsForDel, isForce)
}

// connect to docker
func conn() (*client.Client, error) {
	return client.NewEnvClient()
}

// group images by repository
func groupImages(ctx context.Context, cli *client.Client, isTrimByName bool, tName string) map[string]dockerImages {
	filter := filters.NewArgs()
	if isTrimByName {
		filter.Add("reference", tName)
	}

	imageList, err := cli.ImageList(ctx, types.ImageListOptions{
		Filters: filter,
	})
	if err != nil {
		panic(err)
	}

	tagSplit := ":"
	// imageRepository: []dockerImage{}
	images := make(map[string]dockerImages, 0)
	for _, image := range imageList {
		if len(image.RepoTags) > 1 {
			for _, repoTag := range image.RepoTags {
				rt := strings.Split(repoTag, tagSplit)
				repo, tag := rt[0], rt[1]
				if _, ok := images[repo]; !ok {
					images[repo] = make(dockerImages, 0)
				}
				images[repo] = append(images[repo], dockerImage{
					ID:  image.ID,
					Tag: tag,
				})
			}
		} else {
			rt := strings.Split(image.RepoTags[0], tagSplit)
			repo, tag := rt[0], rt[1]
			if _, ok := images[repo]; !ok {
				images[repo] = make(dockerImages, 0)
			}
			images[repo] = append(images[repo], dockerImage{
				ID:  image.ID,
				Tag: tag,
			})
		}
	}

	return images
}

// filter images by time and number, time has higher priority
func filterImages(images map[string]dockerImages, tNum int64, tTime string, isTrimByNum, isTrimByTime bool) []string {
	var idsForDel []string
	idSplit := ":"

	for _, v := range images {
		sort.Sort(v)
		vLen := int64(v.Len())
		isContinue := isTrimByTime || isTrimByNum
		for idx, di := range v {
			id := strings.Split(di.ID, idSplit)[1]
			if isContinue {
				if isTrimByTime && di.Tag < tTime {
					idsForDel = append(idsForDel, id)
					isContinue = false
					// time has higher priority
					continue
				}
				if isTrimByNum && vLen > tNum && int64(idx) == tNum-1 {
					isContinue = false
				}
			} else {
				idsForDel = append(idsForDel, id)
			}
		}
	}

	return idsForDel
}

func cleanImages(ctx context.Context, cli *client.Client, imageIds []string, isForce bool) {
	options := types.ImageRemoveOptions{
		Force: isForce,
	}

	var idsDeleted []string

	for _, id := range imageIds {
		fmt.Printf("Delete image: %s ... ", id)
		if isForce && stringInArray(idsDeleted, id) {
			printlnWithColor("skip", TextYellow)
			continue
		}

		_, err := cli.ImageRemove(ctx, id, options)
		if err != nil {
			text := fmt.Sprintf("Err %v", err)
			printlnWithColor(text, TextRed)
		} else {
			printlnWithColor("success", TextGreen)
		}

		idsDeleted = append(idsDeleted, id)
	}
}

func pruneImages(ctx context.Context, cli *client.Client) error {
	_, err := cli.ImagesPrune(ctx, filters.NewArgs())
	return err
}

func printWithColor(text string, color int) {
	fmt.Print(colorText(text, color))
}

func printlnWithColor(text string, color int) {
	printWithColor(text, color)
	fmt.Println()
}

func colorText(text string, color int) string {
	return fmt.Sprintf("\033[%dm%s\033[0m", color, text)
}

func stringInArray(arr []string, str string) bool {
	for _, v := range arr {
		if v == str {
			return true
		}
	}

	return false
}

func usage() {
	fmt.Println(`icleaner - clean docker images with filters

Usage: icleaner [-n number] [-t time] [-i image name] [-f]

Tips:
  Supposed to keep the most newest '-n' images which name like '-i' and create time after '-t'. 
  Usually, '-i' has the highest priority, '-t' has the second and '-n' has the last.
  If use '-f', all images with the same ${docker_image_id} will be removed, '-n' '-t' and '-i' will not work at this time.

Options:`)
	flag.PrintDefaults()
}