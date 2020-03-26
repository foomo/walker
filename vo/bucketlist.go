package vo

import "time"

type Bucket struct {
	Name string
	From time.Duration
	To   time.Duration
}

type BucketList []Bucket

func GetBucketList() BucketList {
	return BucketList{
		Bucket{
			Name: "awesome",
			From: time.Duration(time.Millisecond * 0),
			To:   time.Duration(time.Millisecond * 50),
		},
		Bucket{
			Name: "great",
			From: time.Duration(time.Millisecond * 50),
			To:   time.Duration(time.Millisecond * 100),
		},
		Bucket{
			Name: "ok, google loves you",
			From: time.Duration(time.Millisecond * 100),
			To:   time.Duration(time.Millisecond * 200),
		},
		Bucket{
			Name: "not too good, but still ok",
			From: time.Duration(time.Millisecond * 200),
			To:   time.Duration(time.Millisecond * 300),
		},
		Bucket{
			Name: "not great",
			From: time.Duration(time.Millisecond * 300),
			To:   time.Duration(time.Millisecond * 500),
		},
		Bucket{
			Name: "bad, users start to feel a real difference",
			From: time.Duration(time.Millisecond * 500),
			To:   time.Duration(time.Millisecond * 1000),
		},
		Bucket{
			Name: "really bad, you are loosing users",
			From: time.Duration(time.Millisecond * 1000),
			To:   time.Duration(time.Millisecond * 3000),
		},
		Bucket{
			Name: "ouch this seems broken",
			From: time.Duration(time.Millisecond * 3000),
			To:   time.Duration(time.Millisecond * 5000),
		},
		Bucket{
			Name: "catastrophic you site seems to be down",
			From: time.Duration(time.Millisecond * 5000),
			To:   time.Duration(time.Millisecond * 10000),
		},
		Bucket{
			Name: "end of the world - this must not happen",
			From: time.Duration(time.Millisecond * 10000),
			To:   time.Duration(time.Hour),
		},
	}
}
