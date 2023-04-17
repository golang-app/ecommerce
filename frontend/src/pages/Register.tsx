import { AppNavBar, Footer } from "../tetris";
import { RegisterForm } from "../auth";
import { Container } from "@mui/material";

export interface RegisterPageProps {
    onSignUp: (username: string, password: string) => void;
}

export function RegisterPage(props: RegisterPageProps) {
  return (
    <div>
      <AppNavBar loggedIn={false} cartItems={[]} />
      <Container>
        <RegisterForm onSignUp={props.onSignUp}/>
      </Container>
      <Footer />
    </div>
  );
}
