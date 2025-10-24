import React from 'react';
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import Button from '@mui/material/Button';
import IconButton from '@mui/material/IconButton';
import CloseIcon from '@mui/icons-material/Close';
import Typography from '@mui/material/Typography';
import CardMedia from '@mui/material/CardMedia';
import Box from '@mui/material/Box';
import type { SearchResult } from './SearchResultsList';

interface SearchResultDialogProps {
  open: boolean;
  onClose: () => void;
  result: SearchResult | null;
  onDownload: (result: SearchResult) => void;
}

const SearchResultDialog: React.FC<SearchResultDialogProps> = ({ open, onClose, result, onDownload }) => {
  if (!result) return null;
  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <span>{result.mediaName}</span>
        <IconButton onClick={onClose} size="small">
          <CloseIcon />
        </IconButton>
      </DialogTitle>
      <DialogContent>
        <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 2 }}>
          <CardMedia
            component="img"
            sx={{ width: 120, height: 160, objectFit: 'cover', mb: 1 }}
            image={result.imageUrl}
            alt={result.mediaName}
          />
          {result.metadata && (
            <Box>
              <Typography variant="body2">Episodes: {result.metadata.episodes?.length ?? 0}</Typography>
            </Box>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={() => onDownload(result)} variant="contained" color="primary">Download</Button>
      </DialogActions>
    </Dialog>
  );
};

export default SearchResultDialog;
