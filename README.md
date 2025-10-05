# Bibliography CLI

This project is a single golang binary (`./bin/bib`) which uses git to manage bibliography resources (APAv7).
The tool integrates with OpenAI to help lookup resources (so you don't have to do as much typing).  OpenAI integration
replaces the older lookup methods.

<img src="./docs/logo.png" />

## To Use:
- Fork the repo for your own use.
- Build the software (`make`) 
- Run `./bin/bib help` for usage info (you need an OpenAI API Key in your local env vars).
  WARNING: DO NOT PUT YOUR OPENAI API KEY IN THIS REPO!

## To Build:
`make`

## Data Storage
- Citation data is stored in YAML files found in `data/citations/`
- Metadata (e.g., keyword index) is in `data/metadata/`
- When new resources are added using `bin/bib` they are committed to your fork of the git repo.
