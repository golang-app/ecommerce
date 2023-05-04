import AppBar from "@mui/material/AppBar";
import Toolbar from "@mui/material/Toolbar";
import Container from "@mui/material/Container";
import { CartIcon } from "../cart/CartIcon";
import AccountMenu from "./AccountMenu";
import { useCart } from "../cart/useCart";
import { useNavigate } from 'react-router-dom';
import HomeIcon from '@mui/icons-material/Home';
import styled from "@emotion/styled";
import Typography from "@mui/material/Typography";

export interface AppNavBarProps {
  loggedIn: boolean;
}

const StyledHomeIcon = styled(HomeIcon)({
  cursor: "pointer"
});

export function AppNavBar(props: AppNavBarProps): JSX.Element {
  const navigate = useNavigate();
  const cart = useCart();

  console.log("AppNavBar: ", cart)
  
  return (
    <AppBar position="static">
      <Container maxWidth="xl">
        <Toolbar>
          <StyledHomeIcon onClick={() => {navigate("/")}}/>
          <Typography sx={{ flexGrow: 1 }}></Typography>
          
          <CartIcon items={cart?.products} itemsCount={cart?.noOfItems} />
          <AccountMenu loggedIn={props.loggedIn} 
          onLoginClick={function (): void {
             navigate('/login')
          } } 
          onRegisterClick={function (): void {
            navigate('/register')
          } } 
          onLogoutClick={function (): void {
            throw new Error("Function not implemented.");
          } } 
          onSettingsClick={function (): void {
            throw new Error("Function not implemented.");
          } } />
        </Toolbar>
      </Container>
    </AppBar>
  );
}
