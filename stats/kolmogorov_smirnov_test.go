package stats

import "testing"

// TestKolmogorovSmirnovTestRejection contains integration tests with real-world
// response time distributions from online training.
func TestKolmogorovSmirnovTestRejection(t *testing.T) {
	type args struct {
		control    []float64
		candidate  []float64
		percentile Percentile
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Insignificant difference in response times (p95 = 4.262 vs 4.407)",
			args: args{
				control:    []float64{0.007, 0.008, 0.008, 0.01, 0.01, 0.01, 0.011, 0.011, 0.011, 0.012, 0.012, 0.012, 0.013, 0.014, 0.015, 0.015, 0.015, 0.016, 0.017, 0.017, 0.019, 0.02, 0.021, 0.021, 0.022, 0.022, 0.022, 0.022, 0.024, 0.024, 0.024, 0.024, 0.024, 0.024, 0.025, 0.025, 0.026, 0.026, 0.027, 0.027, 0.028, 0.028, 0.028, 0.029, 0.03, 0.03, 0.032, 0.032, 0.035, 0.039, 0.043, 0.046, 0.05, 0.054, 0.055, 0.055, 0.056, 0.056, 0.056, 0.057, 0.057, 0.058, 0.058, 0.058, 0.06, 0.061, 0.061, 0.064, 0.064, 0.064, 0.068, 0.068, 0.072, 0.072, 0.073, 0.073, 0.092, 1.04, 1.512, 1.772, 2.104, 2.236, 2.395, 2.809, 2.988, 3.04, 3.285, 3.451, 3.889, 3.908, 3.975, 4.035, 4.128, 4.254, 4.262, 4.516, 4.583, 4.645, 4.843, 6.886},
				candidate:  []float64{0.006, 0.025, 0.049, 0.027, 0.036, 0.073, 4.036, 2.177, 0.047, 0.047, 0.076, 3.454, 0.013, 0.017, 0.020, 0.008, 0.008, 0.059, 0.059, 0.011, 1.760, 2.531, 1.919, 0.015, 0.054, 0.059, 0.020, 0.294, 1.412, 0.024, 0.032, 0.031, 0.073, 0.079, 0.000, 0.154, 0.334, 0.359, 2.451, 2.211, 0.824, 0.866, 0.866, 0.569, 0.600, 0.385, 2.560, 0.024, 0.030, 0.068, 2.168, 3.789, 0.014, 0.014, 0.010, 0.052, 0.060, 0.066, 0.066, 0.015, 0.020, 0.024, 5.045, 0.018, 0.020, 0.020, 0.029, 3.016, 3.988, 0.010, 0.012, 0.052, 0.056, 0.010, 0.010, 0.010, 4.053, 0.015, 0.020, 0.063, 0.068, 0.009, 0.009, 0.014, 6.256, 4.900, 4.407, 5.232, 6.499, 0.021, 0.023, 0.064, 3.737, 0.015, 0.015, 0.015, 0.016, 0.016, 0.019, 0.008},
				percentile: P90,
			},
			want: false,
		},
		{
			name: "Significant difference in response times (p95 = 5.056 vs 2.868)",
			args: args{
				control:    []float64{0.006, 0.008, 0.008, 0.009, 0.009, 0.01, 0.011, 0.011, 0.012, 0.013, 0.013, 0.013, 0.013, 0.014, 0.014, 0.014, 0.014, 0.014, 0.015, 0.015, 0.015, 0.015, 0.016, 0.016, 0.017, 0.018, 0.018, 0.019, 0.019, 0.019, 0.019, 0.02, 0.021, 0.023, 0.025, 0.025, 0.025, 0.025, 0.025, 0.025, 0.027, 0.029, 0.03, 0.032, 0.033, 0.033, 0.036, 0.036, 0.037, 0.037, 0.038, 0.048, 0.049, 0.051, 0.052, 0.053, 0.054, 0.055, 0.055, 0.055, 0.056, 0.056, 0.06, 0.06, 0.06, 0.063, 0.064, 0.067, 0.068, 0.071, 0.071, 0.072, 0.083, 0.089, 0.487, 0.732, 1.046, 1.391, 2.188, 2.285, 2.365, 2.404, 2.716, 2.909, 3.346, 3.599, 3.625, 3.669, 4.202, 4.269, 4.309, 4.69, 4.813, 4.954, 5.056, 5.128, 5.159, 5.212, 5.309, 8.811},
				candidate:  []float64{0.005, 0.006, 0.006, 0.006, 0.007, 0.007, 0.008, 0.009, 0.01, 0.01, 0.01, 0.01, 0.01, 0.011, 0.011, 0.011, 0.012, 0.012, 0.012, 0.012, 0.013, 0.013, 0.013, 0.013, 0.013, 0.013, 0.013, 0.013, 0.014, 0.014, 0.014, 0.014, 0.014, 0.015, 0.016, 0.016, 0.016, 0.017, 0.018, 0.018, 0.018, 0.019, 0.019, 0.019, 0.02, 0.02, 0.02, 0.02, 0.02, 0.02, 0.021, 0.021, 0.022, 0.024, 0.024, 0.024, 0.024, 0.024, 0.026, 0.026, 0.027, 0.028, 0.029, 0.029, 0.031, 0.031, 0.032, 0.036, 0.047, 0.048, 0.052, 0.054, 0.055, 0.056, 0.056, 0.056, 0.057, 0.058, 0.06, 0.072, 0.074, 0.266, 0.493, 1.117, 1.414, 1.534, 1.794, 1.976, 2.134, 2.336, 2.401, 2.411, 2.62, 2.629, 2.868, 2.873, 3.017, 3.197, 4.092, 7.617},
				percentile: P95,
			},
			want: true,
		},
		{
			name: "Significant difference in response times reversed (p95 = 2.868 vs 5.056)",
			args: args{
				control:    []float64{0.005, 0.006, 0.006, 0.006, 0.007, 0.007, 0.008, 0.009, 0.01, 0.01, 0.01, 0.01, 0.01, 0.011, 0.011, 0.011, 0.012, 0.012, 0.012, 0.012, 0.013, 0.013, 0.013, 0.013, 0.013, 0.013, 0.013, 0.013, 0.014, 0.014, 0.014, 0.014, 0.014, 0.015, 0.016, 0.016, 0.016, 0.017, 0.018, 0.018, 0.018, 0.019, 0.019, 0.019, 0.02, 0.02, 0.02, 0.02, 0.02, 0.02, 0.021, 0.021, 0.022, 0.024, 0.024, 0.024, 0.024, 0.024, 0.026, 0.026, 0.027, 0.028, 0.029, 0.029, 0.031, 0.031, 0.032, 0.036, 0.047, 0.048, 0.052, 0.054, 0.055, 0.056, 0.056, 0.056, 0.057, 0.058, 0.06, 0.072, 0.074, 0.266, 0.493, 1.117, 1.414, 1.534, 1.794, 1.976, 2.134, 2.336, 2.401, 2.411, 2.62, 2.629, 2.868, 2.873, 3.017, 3.197, 4.092, 7.617},
				candidate:  []float64{0.006, 0.008, 0.008, 0.009, 0.009, 0.01, 0.011, 0.011, 0.012, 0.013, 0.013, 0.013, 0.013, 0.014, 0.014, 0.014, 0.014, 0.014, 0.015, 0.015, 0.015, 0.015, 0.016, 0.016, 0.017, 0.018, 0.018, 0.019, 0.019, 0.019, 0.019, 0.02, 0.021, 0.023, 0.025, 0.025, 0.025, 0.025, 0.025, 0.025, 0.027, 0.029, 0.03, 0.032, 0.033, 0.033, 0.036, 0.036, 0.037, 0.037, 0.038, 0.048, 0.049, 0.051, 0.052, 0.053, 0.054, 0.055, 0.055, 0.055, 0.056, 0.056, 0.06, 0.06, 0.06, 0.063, 0.064, 0.067, 0.068, 0.071, 0.071, 0.072, 0.083, 0.089, 0.487, 0.732, 1.046, 1.391, 2.188, 2.285, 2.365, 2.404, 2.716, 2.909, 3.346, 3.599, 3.625, 3.669, 4.202, 4.269, 4.309, 4.69, 4.813, 4.954, 5.056, 5.128, 5.159, 5.212, 5.309, 8.811},
				percentile: P95,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := KolmogorovSmirnovTestRejection(tt.args.control, tt.args.candidate, tt.args.percentile); got != tt.want {
				t.Errorf("KolmogorovSmirnovTestRejection() = %v, want %v", got, tt.want)
			}
		})
	}
}
