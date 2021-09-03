package main

import (
	"github.com/la5nta/pat/internal/cmsapi"
	"net/url"
	"reflect"
	"testing"
)

func Test_toURL(t *testing.T) {
	type args struct {
		channel    cmsapi.GatewayChannel
		targetCall string
	}
	tests := []struct {
		name string
		args args
		want *url.URL
	}{
		{
			name: "ax25 1200",
			args: args{
				channel: cmsapi.GatewayChannel{
					Frequency:      145050000,
					SupportedModes: "Packet 1200",
				},
				targetCall: "K0NTS-10",
			},
			want: parseUrl("ax25:///K0NTS-10?freq=145050"),
		},
		{
			name: "ax25 9600",
			args: args{
				channel: cmsapi.GatewayChannel{
					Frequency:      438075000,
					SupportedModes: "Packet 9600",
				},
				targetCall: "HB9AK-14",
			},
			want: parseUrl("ax25:///HB9AK-14?freq=438075"),
		},
		{
			name: "adrop 2000",
			args: args{
				channel: cmsapi.GatewayChannel{
					Frequency:      3586500,
					SupportedModes: "ARDOP 2000",
				},
				targetCall: "K0SI",
			},
			want: parseUrl("ardop:///K0SI?bw=2000MAX&freq=3585"),
		},
		{
			name: "adrop 500",
			args: args{
				channel: cmsapi.GatewayChannel{
					Frequency:      3584000,
					SupportedModes: "ARDOP 500",
				},
				targetCall: "F1ZWL",
			},
			want: parseUrl("ardop:///F1ZWL?bw=500MAX&freq=3582.5"),
		},
		{
			// These are quite rare but are seen in the wild
			name: "adrop 1000",
			args: args{
				channel: cmsapi.GatewayChannel{
					Frequency:      3588000,
					SupportedModes: "ARDOP 1000",
				},
				targetCall: "N3HYM-10",
			},
			want: parseUrl("ardop:///N3HYM-10?bw=1000MAX&freq=3586.5"),
		},
		{
			// This is a notional ARDOP station that doesn't specify bandwidth in supportedModes.
			// None appear today in the RMS list, but maybe they could.
			name: "adrop unspec",
			args: args{
				channel: cmsapi.GatewayChannel{
					Frequency:      7584000,
					SupportedModes: "ARDOP",
				},
				targetCall: "T3ST",
			},
			want: parseUrl("ardop:///T3ST?freq=7582.5"),
		},
		{
			name: "pactor",
			args: args{
				channel: cmsapi.GatewayChannel{
					Frequency:      1850000,
					SupportedModes: "Pactor 1,2",
				},
				targetCall: "K1EHZ",
			},
			want: parseUrl("pactor:///K1EHZ?freq=1848.5"),
		},
		{
			name: "robust packet",
			args: args{
				channel: cmsapi.GatewayChannel{
					Frequency:      7099400,
					SupportedModes: "Robust Packet",
				},
				targetCall: "N3HYM-10",
			},
			want: parseUrl("ax25:///N3HYM-10?freq=7099.4"),
		},
		{
			name: "vara hf 500",
			args: args{
				channel: cmsapi.GatewayChannel{
					Frequency:      7064000,
					SupportedModes: "VARA 500",
				},
				targetCall: "W0VG",
			},
			want: parseUrl("varahf:///W0VG?bw=500&freq=7062.5"),
		},
		{
			name: "vara hf unspec",
			args: args{
				channel: cmsapi.GatewayChannel{
					Frequency:      7103000,
					SupportedModes: "VARA",
				},
				targetCall: "W0VG",
			},
			want: parseUrl("varahf:///W0VG?bw=2300&freq=7101.5"),
		},
		{
			name: "vara hf 2750",
			args: args{
				channel: cmsapi.GatewayChannel{
					Frequency:      3597900,
					SupportedModes: "VARA 2750",
				},
				targetCall: "W1EO",
			},
			want: parseUrl("varahf:///W1EO?bw=2750&freq=3596.4"),
		},
		{
			name: "vara fm narrow",
			args: args{
				channel: cmsapi.GatewayChannel{
					Frequency:      145070000,
					SupportedModes: "VARA FM",
				},
				targetCall: "W0TQ",
			},
			// vara transport adapter will default to narrow
			want: parseUrl("varafm:///W0TQ?freq=145070"),
		},
		{
			name: "vara fm wide",
			args: args{
				channel: cmsapi.GatewayChannel{
					Frequency:      145510000,
					SupportedModes: "VARA FM WIDE",
				},
				targetCall: "W1AW-10",
			},
			want: parseUrl("varafm:///W1AW-10?freq=145510"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toURL(tt.args.channel, tt.args.targetCall); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("toURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func parseUrl(str string) *url.URL {
	parse, _ := url.Parse(str)
	return parse
}
