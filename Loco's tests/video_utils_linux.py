# Extract frames from video
# python ./video_utils.py -action=extract video.mp4 frames

# Join

import argparse
import cv2
import glob
import numpy as np
import os


def extract_frames(video, frames_path):
    vidcap = cv2.VideoCapture(video)
    success,image = vidcap.read()
    count = 0

    if not os.path.isdir(frames_path):
        os.mkdir(frames_path)

    while success:
        cv2.imwrite("{}/{}.png".format(frames_path,count), image)     # save frame as PNG file
        success,image = vidcap.read()
        print ('Read a new frame: ', count)
        count += 1

def join_frames(frames_path, output):
    if not os.path.isdir(frames_path):
        print(f"'{frames_path}' frames path doesn't exist")
        return

    frames = glob.glob(f'{frames_path}/*.png')

    if not frames:
        print("No frames found")
        return

    frames.sort(key=lambda x: int(os.path.splitext(os.path.basename(x))[0]))

    first = cv2.imread(frames[0])

    if first is None:
        print("Could not read first frame")
        return

    height, width, layers = first.shape
    size = (width, height)

    fourcc = cv2.VideoWriter_fourcc(*'mp4v')
    out = cv2.VideoWriter(output, fourcc, 30.0, size)

    for filename in frames:
        print("Joining frame:", filename)

        img = cv2.imread(filename)

        if img is None:
            print("Skipping bad frame:", filename)
            continue

        out.write(img)

    out.release()

if __name__ == "__main__":

    parser = argparse.ArgumentParser()
    parser.add_argument("-action", default="extract", help="extract or join video frames")
    parser.add_argument("video", default="video.mp4", help="input or output video path")
    parser.add_argument("frames_path", default="frames", help="frames path path")

    args = parser.parse_args()
    if args.action == 'extract':
        extract_frames(args.video, args.frames_path)
    elif args.action == 'join':
        join_frames(args.frames_path, args.video)
    else:
        parser.print_help()
