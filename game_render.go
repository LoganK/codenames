package codenames

import (
	"flag"
	"fmt"
	"image"
	"image/draw"
	_ "image/png"
	"io/ioutil"
	"log"
	"os"
	"sync"

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/math/fixed"
)

var (
	blackCardImage   = flag.String("black_card_image", "frontend/killer.png", "Image file for the black team card")
	blueCardImage    = flag.String("blue_card_image", "frontend/blue_agent.png", "Image file for the blue team card")
	fieldImage       = flag.String("field_image", "frontend/field.png", "Image file for the field background")
	neutralCardImage = flag.String("neutral_card_image", "frontend/neutral.png", "Image file for the neutral team card")
	redCardImage     = flag.String("red_card_image", "frontend/red_agent.png", "Image file for the red team card")

	fontFile = flag.String("font", "frontend/Carlito-Bold.ttf", "Font to use when rendering as an image")

	// The top-left corner of each word box.
	fieldWordPoints = []image.Point{
		image.Point{23, 74},
		image.Point{223, 74},
		image.Point{423, 74},
		image.Point{623, 74},
		image.Point{823, 74},
		image.Point{23, 207},
		image.Point{223, 207},
		image.Point{423, 207},
		image.Point{623, 207},
		image.Point{823, 207},
		image.Point{23, 340},
		image.Point{223, 340},
		image.Point{423, 340},
		image.Point{623, 340},
		image.Point{823, 340},
		image.Point{23, 473},
		image.Point{223, 473},
		image.Point{423, 473},
		image.Point{623, 473},
		image.Point{823, 473},
		image.Point{23, 606},
		image.Point{223, 606},
		image.Point{423, 606},
		image.Point{623, 606},
		image.Point{823, 606},
	}

	cardPoints = []image.Point{
		image.Point{0, 0},
		image.Point{200, 0},
		image.Point{400, 0},
		image.Point{600, 0},
		image.Point{800, 0},
		image.Point{0, 133},
		image.Point{200, 133},
		image.Point{400, 133},
		image.Point{600, 133},
		image.Point{800, 133},
		image.Point{0, 266},
		image.Point{200, 266},
		image.Point{400, 266},
		image.Point{600, 266},
		image.Point{800, 266},
		image.Point{0, 399},
		image.Point{200, 399},
		image.Point{400, 399},
		image.Point{600, 399},
		image.Point{800, 399},
		image.Point{0, 532},
		image.Point{200, 532},
		image.Point{400, 532},
		image.Point{600, 532},
		image.Point{800, 532},
	}
)

const (
	fontSizePoints = float64(20.0)
)

var imgCache = struct {
	sync.Mutex
	forFile map[string]image.Image
}{
	forFile: make(map[string]image.Image),
}

func loadImage(filename string) (image.Image, error) {
	// Allows for double-initialization, but there's no harm there.
	img, ok := imgCache.forFile[filename]
	if ok {
		return img, nil
	}

	reader, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	image, _, err := image.Decode(reader)
	if err != nil {
		return nil, err
	}

	imgCache.Lock()
	defer imgCache.Unlock()
	imgCache.forFile[filename] = image
	return imgCache.forFile[filename], nil
}

func newFontContext() (*freetype.Context, error) {
	var once sync.Once
	var font *truetype.Font
	once.Do(func() {
		fontBytes, err := ioutil.ReadFile(*fontFile)
		if err != nil {
			log.Fatalf("failed to load render font: %v", err)
		}

		font, err = freetype.ParseFont(fontBytes)
		if err != nil {
			log.Fatalf("failed to parse render font: %v", err)
		}
	})

	ctx := freetype.NewContext()
	ctx.SetFont(font)
	ctx.SetFontSize(fontSizePoints)
	ctx.SetSrc(image.Black)

	return ctx, nil
}

func (g *Game) RenderGameBoard() (image.Image, error) {
	imgBoard := image.NewRGBA(image.Rect(0, 0, 1000, 665))

	field, err := loadImage(*fieldImage)
	if err != nil {
		log.Printf("failed to open image: %v", err)
		field = image.White
	}
	draw.Draw(imgBoard, field.Bounds(), field, image.ZP, draw.Over)

	for i, revealed := range g.Revealed {
		if !revealed {
			continue
		}

		var cardFile string
		switch g.Layout[i] {
		case Neutral:
			cardFile = *neutralCardImage
		case Red:
			cardFile = *redCardImage
		case Blue:
			cardFile = *blueCardImage
		case Black:
			cardFile = *blackCardImage
		}
		cardImg, err := loadImage(cardFile)
		if err != nil {
			log.Printf("unable to load card %s: %v", cardFile, err)
			cardImg = image.NewGray(image.Rect(0, 0, 200, 133))
		}

		cardRect := cardImg.Bounds().Add(cardPoints[i])
		draw.Draw(imgBoard, cardRect, cardImg, image.ZP, draw.Over)
	}

	// Note: Non-traditional rendering. Render the text _over_ the card so you can see the "covered" word.

	fontCtx, _ := newFontContext()
	fontRect := image.Rect(0, 0, 153, 37)
	fontCtx.SetClip(fontRect)

	// Vertical center, starts from the bottom of the text.
	yFixed := fixed.Int26_6(int32(fixed.I(fontRect.Dy()))/2 +
		int32(fontCtx.PointToFixed(fontSizePoints))/2)

	for i, word := range g.Words {
		imgWord := image.NewRGBA(fontRect)
		draw.Draw(imgWord, imgWord.Bounds(), image.Transparent, image.ZP, draw.Src)
		fontCtx.SetDst(imgWord)

		// Draw and calculate horizontal center
		leftPt := fixed.Point26_6{
			X: 0,
			Y: yFixed,
		}
		rightPt, err := fontCtx.DrawString(word, leftPt)
		if err != nil {
			return nil, fmt.Errorf("failed to render word: %v", err)
		}
		widthFixed := rightPt.Sub(leftPt).X

		// TODO: Shrink the font size until the word fits (i.e., offset is negative).
		xOffset := (fontRect.Dx() - widthFixed.Ceil()) / 2

		wordRect := fontRect.Add(fieldWordPoints[i]).Add(image.Point{xOffset, 0})

		draw.Draw(imgBoard, wordRect, imgWord, image.ZP, draw.Over)
	}

	return imgBoard, nil
}
