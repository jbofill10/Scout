import React, { useState } from 'react';
import SearchResultDialog from './SearchResultDialog';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import Card from '@mui/material/Card';
import CardContent from '@mui/material/CardContent';
import CardMedia from '@mui/material/CardMedia';

export interface SearchResult {
  id: string;
  imageUrl: string;
  mediaName: string;
  metadata?: {
    episodes?: Array<{
      aired: string;
      id: number;
      image: string;
      name: string;
      number: number;
      seasonNumber: number;
    }>;
  };
}

interface SearchResultsListProps {
  results: SearchResult[];
  mediaType: 'series' | 'movie';
}

const SearchResultsList: React.FC<SearchResultsListProps> = ({ results, mediaType }) => {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [selectedResult, setSelectedResult] = useState<SearchResult | null>(null);

  const handleCardClick = (result: SearchResult) => {
    setSelectedResult(result);
    setDialogOpen(true);
  };

  const handleClose = () => {
    setDialogOpen(false);
    setSelectedResult(null);
  };

  const handleDownload = (result: SearchResult) => {
    // Route to correct endpoint based on media type
    const endpoint = mediaType === 'movie' ? '/api/movies' : '/api/shows';

    fetch(endpoint, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify( result ),
    })
      .then(response => {
        if (!response.ok) {
          throw new Error('Network response was not ok');
        }
        return response.json();
      })
      .then(() => {
        // Download initiated successfully
      })
      .catch(error => {
        console.error('Error initiating download:', error);
      });
  };

  return (
    <>
      <Box sx={{ display: 'flex', flexDirection: 'row', gap: 2, flexWrap: 'wrap', justifyContent: 'center', alignItems: 'center', width: '100%' }}>
        {results.map(result => (
          <Card
            key={result.id}
            sx={{ width: 160, height: 240, display: 'flex', flexDirection: 'column', alignItems: 'center', p: 1, boxSizing: 'border-box', cursor: 'pointer' }}
            onClick={() => handleCardClick(result)}
          >
            <CardMedia
              component="img"
              sx={{ width: 120, height: 160, objectFit: 'cover', mb: 1 }}
              image={result.imageUrl}
              alt={result.mediaName}
            />
            <CardContent sx={{ p: 0, textAlign: 'center', width: '100%', flexGrow: 1, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <Typography variant="subtitle1" sx={{ wordBreak: 'break-word', whiteSpace: 'normal' }}>{result.mediaName}</Typography>
            </CardContent>
          </Card>
        ))}
      </Box>
      <SearchResultDialog
        open={dialogOpen}
        onClose={handleClose}
        result={selectedResult}
        onDownload={handleDownload}
        mediaType={mediaType}
      />
    </>
  );
};

export default SearchResultsList;
