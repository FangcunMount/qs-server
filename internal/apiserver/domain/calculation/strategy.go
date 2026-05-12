package calculation

import "context"

type DefaultStrategyRegistry struct{}

func (DefaultStrategyRegistry) Score(_ context.Context, dimension Dimension, values []float64) (float64, error) {
	switch dimension.StrategyCode {
	case "sum":
		return sumValues(values), nil
	case "avg":
		if len(values) == 0 {
			return 0, nil
		}
		return sumValues(values) / float64(len(values)), nil
	case "cnt":
		return float64(len(values)), nil
	default:
		return 0, nil
	}
}

func sumValues(values []float64) float64 {
	var total float64
	for _, value := range values {
		total += value
	}
	return total
}
