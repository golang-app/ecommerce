import Container from '@mui/material/Container';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';

export function Footer() {
    return (
      <Container maxWidth="xl">
        <Box display="flex" justifyContent="center" alignItems="center" p={2}>
            <Typography variant="body2" color="text.secondary" align="center">
            Footer
            </Typography>
        </Box>
      </Container>
    );
}
