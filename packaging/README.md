# Package sources

These templates generate package files for a tagged release. They are not published automatically.

Pushing a `vX.Y.Z` tag runs the release workflow. It waits for CI, creates the release archives and checksums, and saves the rendered Homebrew formula and Scoop manifest as the `package-files` workflow artifact. It does not publish them to a tap or bucket.

To regenerate files after a release, download its `checksums.txt` and run from the repository root:

```sh
mise exec -- go run ./packaging --version vX.Y.Z --checksums /path/to/checksums.txt --output /path/to/package-output
```

The output contains:

- `homebrew/Formula/easyvenv.rb` for a separate `RisPNG/homebrew-tap` repository.
- `scoop/easyvenv.json` for a separate `RisPNG/scoop-bucket` repository.

After testing the files on their target platforms, copy the formula to `Formula/easyvenv.rb` in the tap and the manifest to `bucket/easyvenv.json` in the bucket, then publish those repositories. Homebrew and Scoop install the commands; users must add shell integration to their profiles to activate environments in the current shell.

Once the tap or bucket is published, update the root README with its install command.
