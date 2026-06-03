# Tests for the Distributed and Parallel Image Processing

The following automation is going to be the one that will be used for
testing your final project. This is not a must but highly recommended
to test your system with this in order to make sure you have
everything in place for the final revision.

## Video Utils

The [`video_utils.py`](./video_utils.py) script is a helper for
extracting hundreds of images from a video. It will provide a high
volume of images that can be processed with your system.

- Install dependencies

```
pip install -r requirements.txt
```

- Download a sample video, for instance [Big Buck Bunny](https://download.blender.org/peach/bigbuckbunny_movies/big_buck_bunny_720p_stereo.avi)

```
curl -Ok https://download.blender.org/peach/bigbuckbunny_movies/big_buck_bunny_720p_stereo.avi
```

- Run the  [`video_utils.py`](video_utils.py) script

```
python video_utils.py -action extract big_buck_bunny_720p_stereo.avi frames
```

The script will generate a new directory `frames` images from the video.



## Test suite

- Login into your system and save the generated `token`

```
curl -u username:password -X POST http://localhost:8080/login
```


- Get Sytem's status

```
curl -H "Authorization: Bearer <ACCESS_TOKEN>" http://localhost:8080/status
```


- Create new workload (save the `workload_id`)

```
curl -X POST -H "Authorization: Bearer <ACCESS_TOKEN>" http://localhost:8080/workloads
```


- Get details about workload

```
curl -H "Authorization: Bearer <ACCESS_TOKEN>" http://localhost:8080/workloads/<workload_id>
```



- Upload images

```
python stress_test.py -action push -workload-id <workload_id> -token <token> -frames-path frames
```


- Get details about workload

```
curl -H "Authorization: Bearer <ACCESS_TOKEN>" http://localhost:8080/workloads/<workload_id>
```


- Download filtered images

```
python3 stress_test.py -action pull -workload-id <workload_id> -image-type filtered -token <token> -frames-path filtered-frames
```


- Join filtered images into a new video

```
python3 video_utils.py -action join filtered.mp4 filtered
```


- Logout from your system
```
curl -X DELETE -H "Authorization: Bearer <ACCESS_TOKEN>" http://localhost:8080/logout
```
