export interface SearchResult {
    id: string,
    imageUrl: string,
    mediaName: string,
    metadata: SearchResultMetadata
    name: string,
    overview: string,
    score: number,
    status: string,
    type: string
    year: string
}

export interface SearchResultMetadata {
    episodes: [Episode],
    firstAired: string,
    lastAired: string,
    number: number,
    score: number
}

export interface Episode {
    aired: string,
    id: number,
    image: string
    name: string
    number: number,
    seasonNumber: number
}