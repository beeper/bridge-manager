package interactive

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/urfave/cli/v2"
)

type Flag struct {
	cli.Flag
	Survey    survey.Prompt
	Validator survey.Validator
	Transform survey.Transformer
}

type settableContext struct {
	*cli.Context
}

func (sc *settableContext) WriteAnswer(field string, value interface{}) error {
	switch typedValue := value.(type) {
	case string:
		return sc.Set(field, typedValue)
	case []string:
		for _, item := range typedValue {
			if err := sc.Set(field, item); err != nil {
				return err
			}
		}
		return nil
	case int, uint, int8, uint8, int16, uint16, int32, uint32, int64, uint64:
		return sc.Set(field, fmt.Sprintf("%d", typedValue))
	default:
		return fmt.Errorf("unsupported type %T", value)
	}
}

func Ask(ctx *cli.Context) error {
	var questions []*survey.Question
	for _, subCtx := range ctx.Lineage() {
		var flags []cli.Flag
		if subCtx.Command != nil {
			flags = subCtx.Command.Flags
		} else if subCtx.App != nil {
			flags = subCtx.App.Flags
		} else {
			return nil
		}
		for _, flag := range flags {
			interactiveFlag, ok := flag.(Flag)
			if !ok || flag.IsSet() || interactiveFlag.Survey == nil {
				continue
			}
			questions = append(questions, &survey.Question{
				Name:      flag.Names()[0],
				Prompt:    interactiveFlag.Survey,
				Validate:  interactiveFlag.Validator,
				Transform: interactiveFlag.Transform,
			})
			var output string
			err := survey.AskOne(interactiveFlag.Survey, &output)
			if err != nil {
				return err
			}
			err = subCtx.Set(flag.Names()[0], output)
			if err != nil {
				return err
			}
		}
	}
	if len(questions) > 0 {
		err := survey.Ask(questions, &settableContext{ctx})
		if err != nil {
			return err
		}
	}
	return nil
}
